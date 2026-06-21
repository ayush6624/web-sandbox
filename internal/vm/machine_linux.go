//go:build linux

package vm

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	fcsdk "github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
)

// Machine wraps the Firecracker SDK machine (Linux only).
type Machine struct {
	*fcsdk.Machine
}

func (o *RunOptions) applyDefaults() error {
	if o.FirecrackerBin == "" {
		o.FirecrackerBin = "firecracker"
	}
	if o.SocketPath == "" {
		id, err := uuid.NewRandom()
		if err != nil {
			return err
		}
		o.SocketPath = filepath.Join(os.TempDir(), fmt.Sprintf("sandbox-%s.sock", id.String()))
	}
	if o.LogDir == "" {
		o.LogDir = os.TempDir()
	}
	return nil
}

func (o RunOptions) fcConfig() (fcsdk.Config, error) {
	if err := o.applyDefaults(); err != nil {
		return fcsdk.Config{}, err
	}

	uid, err := uuid.NewRandom()
	if err != nil {
		return fcsdk.Config{}, err
	}
	logFIFO := filepath.Join(o.LogDir, fmt.Sprintf("sandbox-log-%s.fifo", uid.String()))

	vmID, err := uuid.NewRandom()
	if err != nil {
		return fcsdk.Config{}, err
	}

	drives := []models.Drive{
		{
			DriveID:      fcsdk.String("rootfs"),
			PathOnHost:   fcsdk.String(o.RootfsPath),
			IsRootDevice: fcsdk.Bool(true),
			IsReadOnly:   fcsdk.Bool(false),
		},
	}

	cfg := fcsdk.Config{
		VMID:            vmID.String(),
		SocketPath:      o.SocketPath,
		KernelImagePath: o.KernelImage,
		KernelArgs:      o.KernelArgs,
		Drives:          drives,
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  fcsdk.Int64(o.Vcpus),
			MemSizeMib: fcsdk.Int64(o.MemMIB),
		},
		LogFifo:  logFIFO,
		LogLevel: "Warn",
		Seccomp:  fcsdk.SeccompConfig{Enabled: false},
	}

	if o.TapDevice != "" {
		iface, err := buildNetworkInterface(o)
		if err != nil {
			return fcsdk.Config{}, fmt.Errorf("network config: %w", err)
		}
		cfg.NetworkInterfaces = fcsdk.NetworkInterfaces{iface}
	}

	return cfg, nil
}

func buildNetworkInterface(o RunOptions) (fcsdk.NetworkInterface, error) {
	ip, ipNet, err := net.ParseCIDR(o.GuestCIDR)
	if err != nil {
		return fcsdk.NetworkInterface{}, fmt.Errorf("parse guest CIDR %q: %w", o.GuestCIDR, err)
	}
	ipNet.IP = ip

	gateway := net.ParseIP(o.GatewayIP)
	if gateway == nil {
		return fcsdk.NetworkInterface{}, fmt.Errorf("invalid gateway IP %q", o.GatewayIP)
	}

	var nameservers []string
	if o.Nameservers != "" {
		nameservers = strings.Split(o.Nameservers, ",")
	}

	return fcsdk.NetworkInterface{
		StaticConfiguration: &fcsdk.StaticNetworkConfiguration{
			MacAddress:  o.MacAddress,
			HostDevName: o.TapDevice,
			IPConfiguration: &fcsdk.IPConfiguration{
				IPAddr:      *ipNet,
				Gateway:     gateway,
				Nameservers: nameservers,
				IfName:      "eth0",
			},
		},
	}, nil
}

func buildCommand(ctx context.Context, fcCfg fcsdk.Config, fcBin, logDir string) *exec.Cmd {
	builder := fcsdk.VMCommandBuilder{}.
		WithBin(fcBin).
		WithSocketPath(fcCfg.SocketPath).
		AddArgs("--id", fcCfg.VMID)
	if !fcCfg.Seccomp.Enabled {
		builder = builder.AddArgs("--no-seccomp")
	} else if len(fcCfg.Seccomp.Filter) > 0 {
		builder = builder.AddArgs("--seccomp-filter", fcCfg.Seccomp.Filter)
	}
	cmd := builder.Build(ctx)
	// Capture firecracker's stdout/stderr so we can debug early-exit crashes.
	logPath := filepath.Join(logDir, fmt.Sprintf("firecracker-%s.log", fcCfg.VMID))
	if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); err == nil {
		cmd.Stdout = f
		cmd.Stderr = f
	}
	return cmd
}

func silentLog() *logrus.Entry {
	l := logrus.New()
	l.SetOutput(io.Discard)
	return logrus.NewEntry(l)
}

// NewMachine builds a Machine from RunOptions.
// Pass disableValidation=true to skip SDK path validation (e.g. for dry runs).
func NewMachine(ctx context.Context, opts RunOptions, disableValidation bool) (*Machine, RuntimeConfig, error) {
	fcCfg, err := opts.fcConfig()
	if err != nil {
		return nil, RuntimeConfig{}, err
	}
	fcCfg.DisableValidation = disableValidation

	cmd := buildCommand(ctx, fcCfg, opts.FirecrackerBin, opts.LogDir)
	m, err := fcsdk.NewMachine(ctx, fcCfg, fcsdk.WithProcessRunner(cmd), fcsdk.WithLogger(silentLog()))
	if err != nil {
		return nil, RuntimeConfig{}, err
	}
	rt := RuntimeConfig{SocketPath: fcCfg.SocketPath, VMID: fcCfg.VMID}
	return &Machine{m}, rt, nil
}

// NewMachineFromSnapshot builds a Machine that loads memPath/statePath and
// resumes, instead of cold booting. The network device is restored from the
// snapshot (the SDK's load-snapshot handler list skips network-interface
// creation), so we omit NetworkInterfaces here; the caller must recreate the
// tap under the name baked into the snapshot before Start. The rootfs drive is
// kept only so the SDK's load-snapshot validation can stat it — its contents
// must already match the snapshot's view of the disk.
func NewMachineFromSnapshot(ctx context.Context, opts RunOptions, memPath, statePath string, disableValidation bool) (*Machine, RuntimeConfig, error) {
	opts.TapDevice = "" // device comes from the snapshot; don't add a fresh iface
	fcCfg, err := opts.fcConfig()
	if err != nil {
		return nil, RuntimeConfig{}, err
	}
	fcCfg.DisableValidation = disableValidation

	cmd := buildCommand(ctx, fcCfg, opts.FirecrackerBin, opts.LogDir)
	m, err := fcsdk.NewMachine(ctx, fcCfg,
		fcsdk.WithProcessRunner(cmd),
		fcsdk.WithLogger(silentLog()),
		fcsdk.WithSnapshot(memPath, statePath, func(c *fcsdk.SnapshotConfig) {
			c.ResumeVM = true
		}),
	)
	if err != nil {
		return nil, RuntimeConfig{}, err
	}
	rt := RuntimeConfig{SocketPath: fcCfg.SocketPath, VMID: fcCfg.VMID}
	return &Machine{m}, rt, nil
}

// Start boots the VMM and sends InstanceStart — or, for a snapshot-backed
// machine, loads the snapshot and resumes (the SDK no-ops InstanceStart then).
func Start(ctx context.Context, m *Machine) error {
	if m == nil || m.Machine == nil {
		return fmt.Errorf("nil machine")
	}
	return m.Machine.Start(ctx)
}

// StopForce sends SIGTERM to the Firecracker process (fast teardown).
func StopForce(m *Machine) error {
	if m == nil || m.Machine == nil {
		return nil
	}
	return m.Machine.StopVMM()
}

// ShutdownGuest requests ACPI-style shutdown via CtrlAltDel.
func ShutdownGuest(ctx context.Context, m *Machine) error {
	if m == nil || m.Machine == nil {
		return fmt.Errorf("nil machine")
	}
	return m.Machine.Shutdown(ctx)
}

// Wait blocks until the Firecracker process exits.
func Wait(ctx context.Context, m *Machine) error {
	if m == nil || m.Machine == nil {
		return fmt.Errorf("nil machine")
	}
	return m.Machine.Wait(ctx)
}

// PID returns the Firecracker process PID.
func PID(m *Machine) (int, error) {
	if m == nil || m.Machine == nil {
		return 0, fmt.Errorf("nil machine")
	}
	return m.Machine.PID()
}

// Pause freezes the guest's vCPUs (required before CreateSnapshot).
func Pause(ctx context.Context, m *Machine) error {
	if m == nil || m.Machine == nil {
		return fmt.Errorf("nil machine")
	}
	return m.Machine.PauseVM(ctx)
}

// Resume unfreezes the guest's vCPUs after a snapshot.
func Resume(ctx context.Context, m *Machine) error {
	if m == nil || m.Machine == nil {
		return fmt.Errorf("nil machine")
	}
	return m.Machine.ResumeVM(ctx)
}

// Snapshot writes a full VM snapshot (memory + device state) to the given
// paths. The VM must be paused first.
func Snapshot(ctx context.Context, m *Machine, memPath, statePath string) error {
	if m == nil || m.Machine == nil {
		return fmt.Errorf("nil machine")
	}
	return m.Machine.CreateSnapshot(ctx, memPath, statePath)
}
