package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/ayush6624/web-sandbox/internal/config"
)

func doctorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Validate environment for running devbox VMs",
		RunE:  runDoctor,
	}
	cmd.Flags().StringVar(&cfgPath, "config", "", "path to JSON config (for path-aware checks)")
	cmd.Flags().StringVar(&fcBin, "firecracker", "", "path to firecracker binary (default: search PATH + common locations)")
	return cmd
}

func runDoctor(cmd *cobra.Command, args []string) error {
	var cfg *config.Config
	if cfgPath != "" {
		c, err := config.Load(cfgPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not load config %s: %v\n", cfgPath, err)
		} else {
			cfg = c
		}
	}

	pass, fail, warn := 0, 0, 0
	red := color.New(color.FgRed).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()

	check := func(name string, fn func() (string, error)) {
		detail, err := fn()
		if err != nil {
			fmt.Printf("  %s %s — %v\n", red("✗"), name, err)
			fail++
		} else {
			fmt.Printf("  %s %s%s\n", green("✓"), name, detail)
			pass++
		}
	}
	warnCheck := func(name string, fn func() (string, error)) {
		detail, err := fn()
		if err != nil {
			fmt.Printf("  %s %s — %v\n", yellow("!"), name, err)
			warn++
		} else {
			fmt.Printf("  %s %s%s\n", green("✓"), name, detail)
			pass++
		}
	}

	fmt.Println()
	fmt.Println("websandbox doctor")
	fmt.Println()

	fmt.Println("Platform")
	check("Linux", func() (string, error) {
		if runtime.GOOS != "linux" {
			return "", fmt.Errorf("running on %s (need linux)", runtime.GOOS)
		}
		return fmt.Sprintf(" (%s/%s)", runtime.GOOS, runtime.GOARCH), nil
	})
	check("KVM", func() (string, error) {
		if _, err := os.Stat("/dev/kvm"); err != nil {
			return "", fmt.Errorf("/dev/kvm not accessible — is KVM enabled?")
		}
		return " (/dev/kvm)", nil
	})
	fmt.Println()

	fmt.Println("Firecracker")
	check("Binary", func() (string, error) {
		bin := resolveFcBin(cfg)
		if bin == "" {
			return "", fmt.Errorf("not found in PATH or common locations")
		}
		return fmt.Sprintf(" (%s)", bin), nil
	})
	warnCheck("Version", func() (string, error) {
		bin := resolveFcBin(cfg)
		if bin == "" {
			return "", fmt.Errorf("skipped (no binary)")
		}
		out, err := exec.Command(bin, "--version").CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("could not determine")
		}
		return fmt.Sprintf(" (%s)", strings.TrimSpace(string(out))), nil
	})
	fmt.Println()

	fmt.Println("Guest assets")
	warnCheck("Kernel", func() (string, error) {
		p := pickPath(cfg, func(c *config.Config) string { return c.KernelImage },
			"/opt/fc/vmlinux", "/var/lib/firecracker/vmlinux")
		if p == "" {
			return "", fmt.Errorf("not found at common paths (provide via --config)")
		}
		return fmt.Sprintf(" (%s)", p), nil
	})
	warnCheck("Rootfs", func() (string, error) {
		p := pickPath(cfg, func(c *config.Config) string { return c.RootfsPath },
			"/opt/fc/devbox-rootfs.ext4", "/opt/fc/rootfs.ext4", "/var/lib/firecracker/rootfs.ext4")
		if p == "" {
			return "", fmt.Errorf("not found at common paths — run build-devbox-rootfs.sh")
		}
		return fmt.Sprintf(" (%s)", p), nil
	})
	fmt.Println()

	fmt.Println("Networking")
	tap := "tap0"
	if cfg != nil && cfg.TapDevice != "" {
		tap = cfg.TapDevice
	}
	warnCheck(fmt.Sprintf("Tap device (%s)", tap), func() (string, error) {
		if _, err := os.Stat("/sys/class/net/" + tap); err != nil {
			return "", fmt.Errorf("%s not found — run setup-network.sh", tap)
		}
		return "", nil
	})
	warnCheck("IP forwarding", func() (string, error) {
		b, err := os.ReadFile("/proc/sys/net/ipv4/ip_forward")
		if err != nil {
			return "", fmt.Errorf("could not read ip_forward: %w", err)
		}
		if len(b) == 0 || b[0] != '1' {
			return "", fmt.Errorf("disabled — run: sysctl -w net.ipv4.ip_forward=1")
		}
		return "", nil
	})

	fmt.Println()
	if fail > 0 {
		fmt.Printf("%s, %d passed", red(fmt.Sprintf("%d failed", fail)), pass)
		if warn > 0 {
			fmt.Printf(", %s", yellow(fmt.Sprintf("%d warnings", warn)))
		}
		fmt.Println()
		return fmt.Errorf("%d check(s) failed", fail)
	}
	if warn > 0 {
		fmt.Printf("%s, %s\n", green(fmt.Sprintf("%d passed", pass)), yellow(fmt.Sprintf("%d warnings", warn)))
	} else {
		fmt.Printf("%s\n", green(fmt.Sprintf("All %d checks passed", pass)))
	}
	return nil
}

func resolveFcBin(cfg *config.Config) string {
	if fcBin != "" {
		if _, err := os.Stat(fcBin); err == nil {
			return fcBin
		}
	}
	if cfg != nil && cfg.FirecrackerBin != "" {
		if _, err := os.Stat(cfg.FirecrackerBin); err == nil {
			return cfg.FirecrackerBin
		}
	}
	if p, err := exec.LookPath("firecracker"); err == nil {
		return p
	}
	for _, p := range []string{"/usr/local/bin/firecracker", "/usr/bin/firecracker"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func pickPath(cfg *config.Config, field func(*config.Config) string, fallbacks ...string) string {
	if cfg != nil {
		if p := field(cfg); p != "" {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	for _, p := range fallbacks {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
