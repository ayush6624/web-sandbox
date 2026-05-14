package main

import (
	"fmt"
	"os"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/ayush6624/web-sandbox/internal/state"
)

func downCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "down",
		Short: "Stop a running devbox VM (SIGTERM via state file)",
		RunE:  runDown,
	}
	addVMFlags(cmd)
	return cmd
}

func runDown(cmd *cobra.Command, args []string) error {
	cfg, _, err := loadAndMerge()
	if err != nil {
		return err
	}

	st, err := state.Load(cfg.StatePath)
	if err != nil {
		return fmt.Errorf("no running VM found (state file: %s): %w", cfg.StatePath, err)
	}

	fmt.Printf("Stopping VM %s (pid %d)...\n", st.VMID, st.PID)
	if err := syscall.Kill(st.PID, syscall.SIGTERM); err != nil {
		if err == syscall.ESRCH {
			fmt.Println("Process already gone — clearing state.")
		} else {
			return fmt.Errorf("kill pid %d: %w", st.PID, err)
		}
	}

	if err := os.Remove(cfg.StatePath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "warning: could not remove state: %v\n", err)
	}
	fmt.Println("VM stopped.")
	return nil
}
