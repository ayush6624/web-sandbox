#!/usr/bin/env bash
# GCE startup-script. Runs as root on first boot. Idempotent.
# Reads config from instance metadata (set by vms.sh).
# Output: /var/log/startup-script.log
set -euxo pipefail
exec > >(tee -a /var/log/startup-script.log) 2>&1

meta() {
  curl -fsS -H "Metadata-Flavor: Google" \
    "http://metadata.google.internal/computeMetadata/v1/instance/attributes/$1" 2>/dev/null || true
}

SSH_USER="$(meta ssh-user)"
SSH_USER="${SSH_USER:-ayush}"
TS_KEY="$(meta tailscale-authkey)"
SSH_PUBKEY="$(meta ssh-pubkey)"
# Short instance name (e.g. "testvm-1"), not the long internal FQDN.
INSTANCE_NAME="$(curl -fsS -H "Metadata-Flavor: Google" \
  "http://metadata.google.internal/computeMetadata/v1/instance/name" 2>/dev/null || true)"
INSTANCE_NAME="${INSTANCE_NAME:-$(hostname -s)}"

#############################################
# 1. User with passwordless sudo
#############################################
if ! id "$SSH_USER" &>/dev/null; then
  useradd --create-home --shell /bin/bash "$SSH_USER"
fi

printf '%s ALL=(ALL) NOPASSWD:ALL\n' "$SSH_USER" > "/etc/sudoers.d/90-$SSH_USER"
chmod 0440 "/etc/sudoers.d/90-$SSH_USER"
visudo -cf "/etc/sudoers.d/90-$SSH_USER"

#############################################
# 2. Authorize SSH key (optional)
#############################################
if [ -n "$SSH_PUBKEY" ]; then
  install -d -m 700 -o "$SSH_USER" -g "$SSH_USER" "/home/$SSH_USER/.ssh"
  printf '%s\n' "$SSH_PUBKEY" > "/home/$SSH_USER/.ssh/authorized_keys"
  chmod 600 "/home/$SSH_USER/.ssh/authorized_keys"
  chown "$SSH_USER:$SSH_USER" "/home/$SSH_USER/.ssh/authorized_keys"
fi

#############################################
# 3. Tailscale (optional)
#############################################
if [ -n "$TS_KEY" ]; then
  if ! command -v tailscale &>/dev/null; then
    curl -fsSL https://tailscale.com/install.sh | sh
  fi
  systemctl enable --now tailscaled
  tailscale up --authkey="$TS_KEY" --hostname="$INSTANCE_NAME" --ssh --accept-routes
else
  echo "No tailscale-authkey metadata — skipping Tailscale."
fi

echo "startup-script finished OK"
