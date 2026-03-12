package db

import (
	"context"
	"fmt"
	"os"
	"net"
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

			output.Info(fmt.Sprintf("starting tunnel: localhost:%d → %s:%s:%d", localPort, tunnelHost, addonName, tunnelPort))

			// Start tunnel in background goroutine
			tun := &tunnel.Tunnel{
				TunnelHost:    tunnelHost,
				Password:      password,
				TargetService: addonName,
				TargetPort:    tunnelPort,
				LocalPort:     localPort,
			}

			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			errCh := make(chan error, 1)
			go func() {
				errCh <- tun.Start(ctx)
			}()

			// Wait briefly for listener to be ready
			<-waitReady(localPort)

			output.Info(fmt.Sprintf("tunnel ready — connecting to %s...", addonType))

			err = runNativeClient(addonType, "127.0.0.1", strconv.Itoa(localPort), password, addonName)
			cancel()
			return err
		},
	}
	cmd.Flags().IntVar(&localPort, "local-port", 0, "local port (defaults to service port)")
	return cmd
}

// xquare db tunnel <addon> — starts pure-Go WebSocket tunnel
func newDBTunnelCmd() *cobra.Command {
	var localPort int
	var printURL bool

	cmd := &cobra.Command{
		Use:   "tunnel <addon>",
		Short: "Open a local port tunnel to the database (no external tools needed)",
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

			output.Info(fmt.Sprintf("tunneling localhost:%d → %s:%s:%d", localPort, tunnelHost, addonName, tunnelPort))
			output.Info(fmt.Sprintf("connection: %s", connStr))
			output.Info("press Ctrl+C to stop")

			tun := &tunnel.Tunnel{
				TunnelHost:    tunnelHost,
				Password:      password,
				TargetService: addonName,
				TargetPort:    tunnelPort,
				LocalPort:     localPort,
			}

			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			// Handle Ctrl+C
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
			go func() {
				<-sigCh
				output.Info("\ntunnel closed")
				cancel()
			}()

			return tun.Start(ctx)
		},
	}
	cmd.Flags().IntVar(&localPort, "local-port", 0, "local port (defaults to service port)")
	cmd.Flags().BoolVar(&printURL, "print-url", false, "print connection string only (non-interactive)")
	return cmd
}

// waitReady polls until the local port is listening (up to 2s)
func waitReady(port int) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		for i := 0; i < 20; i++ {
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
			if err == nil {
				conn.Close()
				close(ch)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		close(ch)
	}()
	return ch
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
		return fmt.Errorf("no native client support for %s\nUse 'xquare db tunnel --print-url' for the connection string", addonType)
	}

	if _, err := exec.LookPath(bin); err != nil {
		return fmt.Errorf("%s not found in PATH\nConnection string: %s",
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
