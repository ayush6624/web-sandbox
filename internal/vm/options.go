package vm

// RunOptions configures a microVM run.
type RunOptions struct {
	FirecrackerBin string
	SocketPath     string
	KernelImage    string
	RootfsPath     string
	KernelArgs     string
	Vcpus          int64
	MemMIB         int64
	LogDir         string

	// Networking (optional — if TapDevice is empty, no networking)
	TapDevice   string
	MacAddress  string
	GuestCIDR   string // e.g. "172.16.0.2/24"
	GatewayIP   string // e.g. "172.16.0.1"
	Nameservers string // e.g. "8.8.8.8"

	// Snapshot restore (optional). When both are set, NewMachineFromSnapshot
	// builds a machine that loads this snapshot and resumes instead of cold
	// booting. The network device (incl. tap name, MAC, guest IP) is restored
	// from the snapshot, so the host must recreate the tap under its original
	// name; TapDevice/MacAddress/GuestCIDR are ignored on the restore path.
	SnapshotMemPath   string
	SnapshotStatePath string
}

// RuntimeConfig captures identifiers after the SDK config is built.
type RuntimeConfig struct {
	SocketPath string
	VMID       string
}
