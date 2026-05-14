package main

import (
	"os"

	"github.com/spf13/cobra"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "websandbox",
		Short: "Firecracker-based devbox for React/TS apps",
	}
	root.AddCommand(upCmd(), downCmd(), doctorCmd())
	return root
}
