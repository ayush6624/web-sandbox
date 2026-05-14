package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/ayush6624/web-sandbox/internal/state"
	"github.com/ayush6624/web-sandbox/internal/vm"
)

func upCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Boot a devbox VM (blocks until signal)",
		RunE:  runUp,
	}
	addVMFlags(cmd)
	return cmd
}

func runUp(cmd *cobra.Command, args []string) error {
	cfg, opts, err := loadAndMerge()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Println("Starting devbox VM...")
	m, rt, err := vm.NewMachine(ctx, opts, noValidate)
	if err != nil {
		return fmt.Errorf("create machine: %w", err)
	}
	if err := vm.Start(ctx, m); err != nil {
		return fmt.Errorf("start machine: %w", err)
	}

	pid, err := vm.PID(m)
	if err != nil {
		_ = vm.StopForce(m)
		return fmt.Errorf("get pid: %w", err)
	}

	st := state.VMState{PID: pid, SocketPath: rt.SocketPath, VMID: rt.VMID}
	if err := state.Save(cfg.StatePath, st); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not save state: %v\n", err)
	}

	if opts.TapDevice != "" {
		fmt.Printf("VM running with networking (guest: %s, tap: %s)\n", opts.GuestCIDR, opts.TapDevice)
	} else {
		fmt.Println("VM running (no networking)")
	}
	fmt.Printf("State saved to %s\n", cfg.StatePath)
	fmt.Println("Press Ctrl+C to stop the VM...")

	// Wait for either: signal to us, or the VM process exiting on its own
	// (e.g. crash, internal shutdown, or `websandbox down` SIGTERMing firecracker).
	vmExit := make(chan error, 1)
	go func() { vmExit <- vm.Wait(context.Background(), m) }()

	select {
	case <-ctx.Done():
		fmt.Println("\nStopping VM...")
		shCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_ = vm.ShutdownGuest(shCtx, m)
		cancel()
		select {
		case <-vmExit:
		case <-time.After(2 * time.Minute):
			_ = vm.StopForce(m)
			<-vmExit
		}
	case <-vmExit:
		fmt.Println("\nVM exited.")
	}

	_ = state.Remove(cfg.StatePath)
	fmt.Println("VM stopped.")
	return nil
}
