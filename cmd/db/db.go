package db

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/team-xquare/xquare-cli/internal/api"
	"github.com/team-xquare/xquare-cli/internal/output"
	"github.com/team-xquare/xquare-cli/internal/tunnel"
)

func NewDBCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Database tunnel and connection utilities",
	}
	cmd.AddCommand(
		newDBConnectCmd(),
		newDBTunnelCmd(),
	)
	return cmd
}

// xquare db connect <addon> — starts tunnel then connects using native client
func newDBConnectCmd() *cobra.Command {
	var localPort int

	cmd := &cobra.Command{
		Use:   "connect <addon>",
		Short: "Open an interactive DB session (starts tunnel automatically)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := api.FromCmd(cmd)
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			addonName := args[0]

			conn, err := c.GetAddonConnection(cmd.Context(), project, addonName)
			if err != nil {
				return fmt.Errorf("get connection info: %w", err)
			}

			addonType := fmt.Sprintf("%v", conn["type"])
			tunnelHost := fmt.Sprintf("%v", conn["tunnel_host"])
			tunnelPort := int(conn["tunnel_port"].(float64))
			password := fmt.Sprintf("%v", conn["password"])

			if localPort == 0 {
				localPort = tunnelPort
			}

			wstunnelBin, tmpPath, err := resolveBinary()
			if tmpPath != "" {
				defer os.Remove(tmpPath)
			}
			if err != nil {
				return err
			}

			output.Info(fmt.Sprintf("starting tunnel: localhost:%d → %s:%s:%d", localPort, tunnelHost, addonName, tunnelPort))

			tunnelProc, err := startTunnelProc(wstunnelBin, tunnelHost, password, addonName, tunnelPort, localPort)
			if err != nil {
				return fmt.Errorf("start tunnel: %w", err)
			}
			defer tunnelProc.Kill()

			// Wait for port to be ready
			if err := waitPortReady(localPort, 3*time.Second); err != nil {
				return fmt.Errorf("tunnel did not start in time: %w", err)
			}

			output.Info(fmt.Sprintf("tunnel ready — connecting to %s...", addonType))
			return runNativeClient(addonType, "127.0.0.1", strconv.Itoa(localPort), password, addonName)
		},
	}
	cmd.Flags().IntVar(&localPort, "local-port", 0, "local port (defaults to service port)")
	return cmd
}

// xquare db tunnel <addon> — starts wstunnel port forwarding
func newDBTunnelCmd() *cobra.Command {
	var localPort int
	var printURL bool

	cmd := &cobra.Command{
		Use:   "tunnel <addon>",
		Short: "Open a local port tunnel to the database",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := api.FromCmd(cmd)
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			addonName := args[0]

			conn, err := c.GetAddonConnection(cmd.Context(), project, addonName)
			if err != nil {
				return fmt.Errorf("get connection info: %w", err)
			}

			tunnelHost := fmt.Sprintf("%v", conn["tunnel_host"])
			tunnelPort := int(conn["tunnel_port"].(float64))
			password := fmt.Sprintf("%v", conn["password"])
			addonType := fmt.Sprintf("%v", conn["type"])

			if localPort == 0 {
				localPort = tunnelPort
			}

			connStr := connectionString(addonType, "127.0.0.1", strconv.Itoa(localPort), password, addonName)

			if printURL {
				fmt.Println(connStr)
				return nil
			}

			wstunnelBin, tmpPath, err := resolveBinary()
			if tmpPath != "" {
				defer os.Remove(tmpPath)
			}
			if err != nil {
				return err
			}

			output.Info(fmt.Sprintf("tunneling localhost:%d → %s:%s:%d", localPort, tunnelHost, addonName, tunnelPort))
			output.Info(fmt.Sprintf("connection: %s", connStr))
			output.Info("press Ctrl+C to stop")

			// Handle Ctrl+C
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

			localArg := fmt.Sprintf("tcp://0.0.0.0:%d:%s:%d", localPort, addonName, tunnelPort)
			proc := exec.Command(wstunnelBin, "client",
				"-L", localArg,
				"--http-upgrade-path-prefix", password,
				"--log-lvl", "OFF",
				fmt.Sprintf("wss://%s", tunnelHost),
			)
			proc.Stdout = os.Stdout
			proc.Stderr = os.Stderr
			if err := proc.Start(); err != nil {
				return fmt.Errorf("start wstunnel: %w", err)
			}

			go func() {
				<-sigCh
				output.Info("\ntunnel closed")
				proc.Process.Kill()
			}()

			return proc.Wait()
		},
	}
	cmd.Flags().IntVar(&localPort, "local-port", 0, "local port (defaults to service port)")
	cmd.Flags().BoolVar(&printURL, "print-url", false, "print connection string only (non-interactive)")
	return cmd
}

// resolveBinary returns the wstunnel binary path.
// If it extracts a temp binary, tmpPath is non-empty and must be removed by caller.
func resolveBinary() (binPath, tmpPath string, err error) {
	// Try embedded binary first
	tmp, extractErr := tunnel.ExtractWstunnel()
	if extractErr == nil {
		return tmp, tmp, nil
	}
	// Fallback to system-installed wstunnel
	sys, sysErr := exec.LookPath("wstunnel")
	if sysErr == nil {
		return sys, "", nil
	}
	return "", "", fmt.Errorf("wstunnel not available: %v", extractErr)
}

func startTunnelProc(bin, tunnelHost, password, serviceName string, servicePort, localPort int) (*os.Process, error) {
	localArg := fmt.Sprintf("tcp://0.0.0.0:%d:%s:%d", localPort, serviceName, servicePort)
	cmd := exec.Command(bin, "client",
		"-L", localArg,
		"--http-upgrade-path-prefix", password,
		"--log-lvl", "OFF",
		fmt.Sprintf("wss://%s", tunnelHost),
	)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd.Process, nil
}

func waitPortReady(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("port %d not ready after %s", port, timeout)
}

func runNativeClient(addonType, host, port, password, dbName string) error {
	var args []string
	var bin string

	switch addonType {
	case "mysql":
		bin = "mysql"
		args = []string{"-h", host, "-P", port, "-u", "root", "-p" + password, dbName}
	case "postgresql":
		bin = "psql"
		args = []string{fmt.Sprintf("postgresql://postgres:%s@%s:%s/%s", password, host, port, dbName)}
	case "redis":
		bin = "redis-cli"
		args = []string{"-h", host, "-p", port, "-a", password}
	case "mongodb":
		bin = "mongosh"
		args = []string{fmt.Sprintf("mongodb://root:%s@%s:%s/%s", password, host, port, dbName)}
	default:
		return fmt.Errorf("no native client support for %s\nUse 'xquare db tunnel --print-url'", addonType)
	}

	if _, err := exec.LookPath(bin); err != nil {
		return fmt.Errorf("%s not found in PATH\nConnection: %s",
			bin, connectionString(addonType, host, port, password, dbName))
	}

	c := exec.Command(bin, args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func connectionString(addonType, host, port, password, dbName string) string {
	switch addonType {
	case "mysql":
		return fmt.Sprintf("mysql://root:%s@%s:%s/%s", password, host, port, dbName)
	case "postgresql":
		return fmt.Sprintf("postgresql://postgres:%s@%s:%s/%s", password, host, port, dbName)
	case "redis":
		return fmt.Sprintf("redis://:%s@%s:%s", password, host, port)
	case "mongodb":
		return fmt.Sprintf("mongodb://root:%s@%s:%s/%s", password, host, port, dbName)
	default:
		return fmt.Sprintf("%s://root:%s@%s:%s/%s", addonType, password, host, port, dbName)
	}
}
