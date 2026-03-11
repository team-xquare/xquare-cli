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

// xquare db connect <addon> — starts tunnel then connects using native client
func newDBConnectCmd() *cobra.Command {
	var localPort int
	return &cobra.Command{
		Use:   "connect <addon>",
		Short: "Open an interactive DB session via tunnel (requires native client installed)",
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

			// Check wstunnel is available
			if _, err := exec.LookPath("wstunnel"); err != nil {
				return fmt.Errorf("wstunnel not found in PATH\n\nInstall from: https://github.com/erebe/wstunnel/releases\nOr use Docker: docker run -p %d:%d -e PASSWORD=%s -e SERVICE_NAME=%s -e SERVICE_PORT=%d -e PROJECT_NAME=%s ghcr.io/erebe/wstunnel client ...",
					localPort, tunnelPort, password, addonName, tunnelPort, project)
			}

			// Start wstunnel in background
			localArg := fmt.Sprintf("tcp://127.0.0.1:%d:%s:%d", localPort, addonName, tunnelPort)
			tunnel := exec.Command("wstunnel", "client",
				"-L", localArg,
				"--http-upgrade-path-prefix", password,
				"--log-lvl", "OFF",
				fmt.Sprintf("wss://%s", tunnelHost),
			)
			if err := tunnel.Start(); err != nil {
				return fmt.Errorf("start wstunnel: %w", err)
			}
			defer tunnel.Process.Kill()

			output.Info(fmt.Sprintf("tunnel: localhost:%d → %s:%d", localPort, addonName, tunnelPort))

			return runNativeClient(addonType, "127.0.0.1", strconv.Itoa(localPort), password, addonName)
		},
	}
}

// xquare db tunnel <addon> — starts wstunnel port forwarding
func newDBTunnelCmd() *cobra.Command {
	var localPort int
	var printURL bool

	cmd := &cobra.Command{
		Use:   "tunnel <addon>",
		Short: "Open a local port tunnel to the database (requires wstunnel)",
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

			if _, err := exec.LookPath("wstunnel"); err != nil {
				return fmt.Errorf("wstunnel not found in PATH\n\nInstall from: https://github.com/erebe/wstunnel/releases")
			}

			output.Info(fmt.Sprintf("tunneling localhost:%d → %s:%s:%d", localPort, tunnelHost, addonName, tunnelPort))
			output.Info(fmt.Sprintf("connection string: %s", connStr))
			output.Info("press Ctrl+C to stop")

			return startWSTunnel(tunnelHost, addonName, tunnelPort, localPort, password)
		},
	}
	cmd.Flags().IntVar(&localPort, "local-port", 0, "local port (defaults to service port)")
	cmd.Flags().BoolVar(&printURL, "print-url", false, "print connection string only (non-interactive)")
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
		return fmt.Errorf("no native client support for addon type: %s\nUse 'xquare db tunnel' to set up port forwarding", addonType)
	}

	if _, err := exec.LookPath(bin); err != nil {
		return fmt.Errorf("%s not found in PATH — install it first\nOr use 'xquare db tunnel --print-url' for the connection string", bin)
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
		return fmt.Sprintf("%s://%s:%s@%s:%s", addonType, "root", password, host, port)
	}
}

func startWSTunnel(tunnelHost, serviceName string, servicePort, localPort int, password string) error {
	bin, err := exec.LookPath("wstunnel")
	if err != nil {
		return fmt.Errorf("wstunnel not found in PATH\n\nInstall from: https://github.com/erebe/wstunnel/releases")
	}

	// wstunnel client -L tcp://127.0.0.1:{localPort}:{serviceName}:{servicePort} \
	//   --http-upgrade-path-prefix {password} \
	//   --log-lvl OFF wss://{tunnelHost}
	localArg := fmt.Sprintf("tcp://127.0.0.1:%d:%s:%d", localPort, serviceName, servicePort)

	c := exec.Command(bin, "client",
		"-L", localArg,
		"--http-upgrade-path-prefix", password,
		"--log-lvl", "OFF",
		fmt.Sprintf("wss://%s", tunnelHost),
	)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
