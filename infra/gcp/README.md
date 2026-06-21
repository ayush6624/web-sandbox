# GCP test VMs (gcloud)

Plain `gcloud` scripts to spin up disposable GCE VMs in **Mumbai
(`asia-south1`, GCP's equivalent of AWS `ap-south-1`)** and tear them down when
you're done. Each VM:

- **8 vCPU / 32 GB RAM** (`n2-standard-8`)
- **512 GB SSD** boot disk (`pd-ssd`)
- Ubuntu 24.04 LTS
- **Spot (preemptible)** — much cheaper, reclaimable by GCP at any time
  (toggle `SPOT` in `config.env`)
- **no service account** attached (`--no-service-account --no-scopes`)
- a **`ayush`** user with **passwordless sudo**
- **Tailscale** installed + joined to your tailnet on first boot (with Tailscale SSH)

Defaults: **2** VMs (`testvm-1`, `testvm-2`).

## Prerequisites

```bash
gcloud auth login
gcloud config set project ratio-experiments
gcloud services enable compute.googleapis.com    # one-time
```

## Usage

```bash
cd infra/gcp
cp config.env.example config.env   # config.env is gitignored — keep secrets here
$EDITOR config.env                 # set PROJECT, and your EPHEMERAL TAILSCALE_AUTHKEY

./vms.sh up                 # create the VMs
./vms.sh list               # status + external/internal IPs
./vms.sh ssh testvm-1       # gcloud ssh into one
./vms.sh down               # delete them all (add -y to skip the prompt)
```

## How it works

- **`config.env`** — all the knobs (project, zone, names, machine type, disk,
  user, Tailscale key). Edit this.
- **`vms.sh`** — `up` / `down` / `list` / `ssh` wrappers around `gcloud`. `up`
  creates every name in `NAMES` in a single `gcloud compute instances create`
  call with `--no-service-account --no-scopes`.
- **`startup.sh`** — runs as root on first boot. Reads `ssh-user`,
  `tailscale-authkey`, and `ssh-pubkey` from instance metadata, then creates the
  user with passwordless sudo and brings up Tailscale. Idempotent. Output is
  logged to `/var/log/startup-script.log` on each VM.

The Tailscale key and any SSH key are passed via instance **metadata**, not
baked into the committed script.

## Connecting

- **Over Tailscale (recommended):** once a box appears in your tailnet,
  `ssh ayush@testvm-1`. Tailscale SSH authorizes you by tailnet identity — no
  keys to manage.
- **Direct:** `./vms.sh ssh testvm-1` (uses `gcloud compute ssh`), or set
  `SSH_PUBLIC_KEY` in `config.env` and `ssh ayush@<external-ip>`.

## Tear down

```bash
./vms.sh down            # or: ./vms.sh down -y
```

Deletes the instances (and their boot disks). The Tailscale auth key is
**ephemeral**, so the nodes auto-remove from your tailnet once they go offline —
no manual cleanup needed.

## Notes

- Provisioning happens on first boot, so the user/Tailscale take ~30–60s after
  the VM shows `RUNNING`. Watch it:
  `./vms.sh ssh testvm-1 -- sudo tail -f /var/log/startup-script.log`
- `config.env` holds your project ID and (if set) the Tailscale key — it's
  gitignored.
- Want different counts/specs? Edit `NAMES`, `MACHINE_TYPE`, `DISK_SIZE`, etc.
  in `config.env`.
