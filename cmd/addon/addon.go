package addon

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/team-xquare/xquare-cli/internal/api"
	"github.com/team-xquare/xquare-cli/internal/output"
	"github.com/team-xquare/xquare-cli/internal/tunnel"
)

var storageRe = regexp.MustCompile(`^(\d+)(Ki|Mi|Gi|Ti|Pi|E|P|T|G|M|K)$`)

const maxStorageBytes = 4 * 1024 * 1024 * 1024 // 4Gi

func parseStorageBytes(s string) (int64, error) {
	m := storageRe.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("invalid storage %q: must be a number followed by a unit (e.g. 1Gi, 500Mi)", s)
	}
	n, _ := strconv.ParseInt(m[1], 10, 64)
	units := map[string]int64{
		"Ki": 1024, "Mi": 1024 * 1024, "Gi": 1024 * 1024 * 1024,
		"Ti": 1024 * 1024 * 1024 * 1024, "Pi": 1024 * 1024 * 1024 * 1024 * 1024,
		"K": 1000, "M": 1000 * 1000, "G": 1000 * 1000 * 1000,
		"T": 1000 * 1000 * 1000 * 1000, "P": 1000 * 1000 * 1000 * 1000 * 1000,
		"E": 1000 * 1000 * 1000 * 1000 * 1000 * 1000,
	}
	return n * units[m[2]], nil
}

func NewAddonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "addon",
		Short: "Manage addons (databases, caches, etc.)",
	}
	cmd.AddCommand(
		newAddonListCmd(),
		newAddonCreateCmd(),
		newAddonDeleteCmd(),
		newAddonGetCmd(),
		newAddonConnectCmd(),
		newAddonTunnelCmd(),
	)
	return cmd
}

func newAddonListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List addons in a project",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			c := api.FromCmd(cmd)
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			addons, err := c.ListAddons(cmd.Context(), project)
			if err != nil {
				return err
			}
			if api.IsJSON(cmd) {
				return output.JSON(addons)
			}
			if len(addons) == 0 {
				output.Info("no addons found")
				return nil
			}
			rows := make([][]string, 0, len(addons))
			for _, a := range addons {
				readyStr := "⏳ 프로비저닝 중"
				if fmt.Sprintf("%v", a["ready"]) == "true" {
					readyStr = "✓ 사용 가능"
				}
				rows = append(rows, []string{
					fmt.Sprintf("%v", a["name"]),
					fmt.Sprintf("%v", a["type"]),
					fmt.Sprintf("%v", a["storage"]),
					readyStr,
				})
			}
			output.Table([]string{"NAME", "TYPE", "STORAGE", "STATUS"}, rows)
			return nil
		},
	}
}

func newAddonCreateCmd() *cobra.Command {
	var storage string
	var bootstrap string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "create <name> <type>",
		Short: "Create an addon (mysql, postgresql, redis, mongodb, etc.)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			addonType := args[1]
			validTypes := map[string]bool{
				"mysql": true, "postgresql": true, "redis": true, "mongodb": true,
				"kafka": true, "rabbitmq": true, "opensearch": true, "elasticsearch": true, "qdrant": true,
			}
			if !validTypes[addonType] {
				return fmt.Errorf("unsupported addon type %q\n\nSupported types: mysql, postgresql, redis, mongodb, kafka, rabbitmq, opensearch, elasticsearch, qdrant", addonType)
			}
			storageBytes, err := parseStorageBytes(storage)
			if err != nil {
				return err
			}
			if storageBytes >= maxStorageBytes {
				return fmt.Errorf("storage must be less than 4Gi (got %s)", storage)
			}
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			if dryRun {
				output.Info(fmt.Sprintf("[dry-run] would create %s addon '%s' in project %s", addonType, args[0], project))
				return nil
			}
			c := api.FromCmd(cmd)
			body := map[string]string{
				"name":    args[0],
				"type":    args[1],
				"storage": storage,
			}
			if bootstrap != "" {
				body["bootstrap"] = bootstrap
			}
			result, err := c.CreateAddon(cmd.Context(), project, body)
			if err != nil {
				return err
			}
			if api.IsJSON(cmd) {
				return output.JSON(result)
			}
			output.Success(fmt.Sprintf("created addon '%s' (%s)", args[0], args[1]))
			output.Info("DB 프로비저닝 중... (약 1~2분 소요)")
			output.Info(fmt.Sprintf("  xquare addon list   # 준비 상태 확인"))
			return nil
		},
	}
	cmd.Flags().StringVar(&storage, "storage", "2Gi", "storage size, max 4Gi (e.g. 2Gi, 500Mi)")
	cmd.Flags().StringVar(&bootstrap, "bootstrap", "", "bootstrap SQL/script")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would happen")
	return cmd
}

func newAddonDeleteCmd() *cobra.Command {
	var yes, dryRun bool
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete an addon",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			if dryRun {
				output.Info(fmt.Sprintf("[dry-run] would delete addon '%s' from project %s", args[0], project))
				return nil
			}
			if !yes {
				return fmt.Errorf("use --yes to confirm deletion")
			}
			c := api.FromCmd(cmd)
			if err := c.DeleteAddon(cmd.Context(), project, args[0]); err != nil {
				return err
			}
			output.Success(fmt.Sprintf("deleted addon '%s'", args[0]))
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm deletion")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would happen")
	return cmd
}

// xquare addon get <name> — show connection info (no tunnel hints)
func newAddonGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Show connection info for an addon",
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
				return err
			}
			if api.IsJSON(cmd) {
				return output.JSON(conn)
			}
			ready := fmt.Sprintf("%v", conn["ready"]) == "true"
			host := fmt.Sprintf("%v", conn["host"])
			readyStr := "⏳ 프로비저닝 중"
			if ready {
				readyStr = "✓ 사용 가능"
			}
			addonType := fmt.Sprintf("%v", conn["type"])
			rows := [][]string{
				{"Status", readyStr},
				{"Type", addonType},
				{"Host", host},
				{"Port", fmt.Sprintf("%v", conn["port"])},
				{"Password", fmt.Sprintf("%v", conn["password"])},
			}
			output.Table([]string{"FIELD", "VALUE"}, rows)
			return nil
		},
	}
}

// xquare addon connect <name> — starts tunnel then launches native client
func newAddonConnectCmd() *cobra.Command {
	var localPort int

	cmd := &cobra.Command{
		Use:   "connect <name>",
		Short: "Open an interactive session (starts tunnel automatically)",
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
			tunnelHost := fmt.Sprintf("%v", conn["host"])
			portF, portOK := conn["port"].(float64)
			if !portOK {
				return fmt.Errorf("addon %q is not ready yet\n\n  xquare addon list   # check provisioning status", addonName)
			}
			tunnelPort := int(portF)
			password := fmt.Sprintf("%v", conn["password"])

			if localPort == 0 {
				localPort = freePort(tunnelPort)
			}

			wstunnelBin, cleanupBin, err := resolveBinary()
			if cleanupBin != nil {
				defer cleanupBin()
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

// xquare addon tunnel <name> — starts wstunnel port forwarding only
func newAddonTunnelCmd() *cobra.Command {
	var localPort int
	var printURL bool

	cmd := &cobra.Command{
		Use:   "tunnel <name>",
		Short: "Open a local port tunnel to the addon",
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

			tunnelHost := fmt.Sprintf("%v", conn["host"])
			portF, portOK := conn["port"].(float64)
			if !portOK {
				return fmt.Errorf("addon %q is not ready yet\n\n  xquare addon list   # check provisioning status", addonName)
			}
			tunnelPort := int(portF)
			password := fmt.Sprintf("%v", conn["password"])
			addonType := fmt.Sprintf("%v", conn["type"])

			if localPort == 0 {
				localPort = freePort(tunnelPort)
			}

			connStr := connectionString(addonType, "127.0.0.1", strconv.Itoa(localPort), password, addonName)

			if printURL {
				fmt.Println(connStr)
				return nil
			}

			wstunnelBin, cleanupBin, err := resolveBinary()
			if cleanupBin != nil {
				defer cleanupBin()
			}
			if err != nil {
				return err
			}

			output.Info(fmt.Sprintf("tunneling localhost:%d → %s:%s:%d", localPort, tunnelHost, addonName, tunnelPort))
			output.Info(fmt.Sprintf("connection: %s", connStr))
			output.Info("press Ctrl+C to stop")

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

func resolveBinary() (binPath string, cleanup func(), err error) {
	return tunnel.ExtractWstunnel()
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

// freePort returns preferred if it's available, otherwise finds a random free port.
func freePort(preferred int) int {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", preferred), 200*time.Millisecond)
	if err != nil {
		return preferred // port is free
	}
	conn.Close()
	// preferred is in use — ask OS for a free port
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return preferred
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
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
		args = []string{"-h", host, "-P", port, "-u", "root", dbName}
	case "postgresql":
		bin = "psql"
		args = []string{fmt.Sprintf("postgresql://postgres@%s:%s/%s", host, port, dbName)}
	case "redis":
		bin = "redis-cli"
		args = []string{"-h", host, "-p", port}
	case "mongodb":
		bin = "mongosh"
		args = []string{fmt.Sprintf("mongodb://root@%s:%s/%s", host, port, dbName)}
	default:
		return fmt.Errorf("no native client support for %s\nUse 'xquare addon tunnel --print-url'", addonType)
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

func connectionString(addonType, host, port, _, dbName string) string {
	switch addonType {
	case "mysql":
		return fmt.Sprintf("mysql://root@%s:%s/%s", host, port, dbName)
	case "postgresql":
		return fmt.Sprintf("postgresql://postgres@%s:%s/%s", host, port, dbName)
	case "redis":
		return fmt.Sprintf("redis://%s:%s", host, port)
	case "mongodb":
		return fmt.Sprintf("mongodb://root@%s:%s/%s", host, port, dbName)
	default:
		return fmt.Sprintf("%s://root@%s:%s/%s", addonType, host, port, dbName)
	}
}
