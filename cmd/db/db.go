package db

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/team-xquare/xquare-cli/internal/api"
	"github.com/team-xquare/xquare-cli/internal/output"
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

// xquare db connect <addon> — connects using the native CLI client
func newDBConnectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "connect <addon>",
		Short: "Open an interactive DB session (requires native client installed)",
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
			host := fmt.Sprintf("%v", conn["tunnel_host"])
			port := fmt.Sprintf("%v", conn["tunnel_port"])
			password := fmt.Sprintf("%v", conn["password"])

			output.Info(fmt.Sprintf("connecting to %s %s:%s ...", addonType, host, port))

			return runNativeClient(addonType, host, port, password, addonName)
		},
	}
}

// xquare db tunnel <addon> [local-port] — starts wstunnel-based port forwarding
func newDBTunnelCmd() *cobra.Command {
	var localPort int

	cmd := &cobra.Command{
		Use:   "tunnel <addon> [local-port]",
		Short: "Open a local port tunnel to the database",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := api.FromCmd(cmd)
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			addonName := args[0]
			if len(args) == 2 {
				p, err := strconv.Atoi(args[1])
				if err != nil {
					return fmt.Errorf("invalid port: %s", args[1])
				}
				localPort = p
			}

			conn, err := c.GetAddonConnection(cmd.Context(), project, addonName)
			if err != nil {
				return fmt.Errorf("get connection info: %w", err)
			}

			tunnelHost := fmt.Sprintf("%v", conn["tunnel_host"])
			tunnelPort := int(conn["tunnel_port"].(float64))
			password := fmt.Sprintf("%v", conn["password"])

			if localPort == 0 {
				localPort = tunnelPort
			}

			output.Info(fmt.Sprintf("tunneling %s:%d → localhost:%d", tunnelHost, tunnelPort, localPort))
			output.Info(fmt.Sprintf("password: %s", password))
			output.Info("press Ctrl+C to stop")

			return startWSTunnel(tunnelHost, tunnelPort, localPort, password)
		},
	}
	cmd.Flags().IntVar(&localPort, "local-port", 0, "local port (defaults to service port)")
	return cmd
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
		return fmt.Errorf("no native client support for addon type: %s", addonType)
	}

	if _, err := exec.LookPath(bin); err != nil {
		return fmt.Errorf("%s not found in PATH — install it first", bin)
	}

	c := exec.Command(bin, args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func startWSTunnel(remoteHost string, remotePort, localPort int, _ string) error {
	// wstunnel client mode: forward localhost:localPort → ws://remoteHost → remotePort
	bin, err := exec.LookPath("wstunnel")
	if err != nil {
		return fmt.Errorf("wstunnel not found in PATH\n\nInstall: https://github.com/erebe/wstunnel/releases")
	}

	wsURL := fmt.Sprintf("wss://%s", remoteHost)
	localArg := fmt.Sprintf("tcp://127.0.0.1:%d:127.0.0.1:%d", localPort, remotePort)

	c := exec.Command(bin, "client", "-L", localArg, wsURL)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
