package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/team-xquare/xquare-cli/cmd/addon"
	"github.com/team-xquare/xquare-cli/cmd/app"
	"github.com/team-xquare/xquare-cli/cmd/auth"
	"github.com/team-xquare/xquare-cli/cmd/build"
	"github.com/team-xquare/xquare-cli/cmd/env"
	"github.com/team-xquare/xquare-cli/cmd/logs"
	"github.com/team-xquare/xquare-cli/cmd/mcp"
	"github.com/team-xquare/xquare-cli/cmd/project"
	"github.com/team-xquare/xquare-cli/cmd/schema"
	"github.com/team-xquare/xquare-cli/cmd/server"
	"github.com/team-xquare/xquare-cli/cmd/trigger"
	"github.com/team-xquare/xquare-cli/internal/api"
	"github.com/team-xquare/xquare-cli/internal/config"
	"github.com/team-xquare/xquare-cli/internal/output"
	"github.com/team-xquare/xquare-cli/internal/updater"
)

var cliVersion = "dev"
var cliCommit = "none"
var cliDate = "unknown"

// SetVersion is called from main with values injected by GoReleaser ldflags
func SetVersion(v, c, d string) {
	cliVersion = v
	cliCommit = c
	cliDate = d
}

var rootCmd = &cobra.Command{
	Use:   "xquare",
	Short: "xquare PaaS CLI — manage your projects, apps, and services",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Suppress usage on runtime errors (usage only makes sense for wrong flags/args)
		cmd.SilenceUsage = true
		isJSON, _ := cmd.Root().PersistentFlags().GetBool("json")
		noInput, _ := cmd.Root().PersistentFlags().GetBool("no-input")
		output.SetJSONMode(isJSON)
		output.SetNoInput(noInput)
		jq, _ := cmd.Root().PersistentFlags().GetString("jq")
		fields, _ := cmd.Root().PersistentFlags().GetStringSlice("fields")
		output.SetGlobalFilters(jq, fields)
		// Background version check — only in interactive terminal sessions
		if !isJSON && !noInput && output.IsTTY() {
			updater.CheckForUpdate(cliVersion)
		}
	},
}

func init() {
	rootCmd.PersistentFlags().Bool("json", false, "output as JSON")
	rootCmd.PersistentFlags().String("jq", "", "filter JSON output with a jq expression")
	rootCmd.PersistentFlags().StringSlice("fields", nil, "select fields from JSON response (e.g. name,status)")
	rootCmd.PersistentFlags().StringP("project", "p", "", "project name (overrides XQUARE_PROJECT or .xquare/config)")
	rootCmd.PersistentFlags().Bool("no-input", false, "disable interactive prompts (useful in CI)")
	rootCmd.PersistentFlags().String("server", "", "xquare server URL (overrides XQUARE_SERVER_URL)")

	rootCmd.AddCommand(
		newVersionCmd(),
		newUpgradeCmd(),
		auth.NewAuthCmd(),
		auth.NewLoginCmd(), // keep top-level `xquare login` shortcut
		project.NewProjectCmd(),
		app.NewAppCmd(),
		build.NewBuildCmd(),
		trigger.NewTriggerCmd(),
		server.NewServerCmd(),
		env.NewEnvCmd(),
		addon.NewAddonCmd(),
		logs.NewLogsCmd(),
		mcp.NewMCPCmd(),
		schema.NewSchemaCmd(),
		newLinkCmd(),
		newUnlinkCmd(),
		newWhoamiCmd(),
	)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		if output.IsJSONMode() {
			// Classify error into machine-readable code
			msg := err.Error()
			code := classifyError(msg)
			output.PrintJSONError(code, msg, "")
		}
		os.Exit(1)
	}
}

// classifyError maps common error patterns to machine-readable codes
func classifyError(msg string) string {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "not logged in") || strings.Contains(lower, "unauthorized"):
		return "auth_error"
	case strings.Contains(lower, "not found") || strings.Contains(lower, "404"):
		return "not_found"
	case strings.Contains(lower, "already exists") || strings.Contains(lower, "409") || strings.Contains(lower, "conflict"):
		return "conflict"
	case strings.Contains(lower, "invalid project name"):
		return "invalid_project_name"
	case strings.Contains(lower, "invalid app name"):
		return "invalid_app_name"
	case strings.Contains(lower, "storage") && strings.Contains(lower, "4gi"):
		return "storage_too_large"
	case strings.Contains(lower, "ci_not_ready") || strings.Contains(lower, "ci not ready"):
		return "ci_not_ready"
	case strings.Contains(lower, "timeout"):
		return "timeout"
	case strings.Contains(lower, "server error") || strings.Contains(lower, "500"):
		return "server_error"
	default:
		return "error"
	}
}

func newUpgradeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade xquare CLI to the latest version",
		RunE: func(cmd *cobra.Command, args []string) error {
			return updater.Upgrade(cliVersion)
		},
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			if isJSON, _ := cmd.Root().PersistentFlags().GetBool("json"); isJSON {
				_ = output.JSON(map[string]string{
					"version": cliVersion,
					"commit":  cliCommit,
					"date":    cliDate,
				})
				return
			}
			fmt.Printf("xquare %s (%s) built %s\n", cliVersion, cliCommit[:min(len(cliCommit), 7)], cliDate)
		},
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// xquare link <project>
func newLinkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "link <project>",
		Short: "Link current directory to a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectName := args[0]
			c := api.FromCmd(cmd)
			projects, err := c.ListProjects(cmd.Context())
			if err != nil {
				return fmt.Errorf("verify project: %w", err)
			}
			found := false
			for _, p := range projects {
				if p == projectName {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("project %q not found\n\n  xquare project list       # see all projects\n  xquare project create %s  # create it", projectName, projectName)
			}
			if err := config.SaveProject(&config.ProjectConfig{Project: projectName}); err != nil {
				return fmt.Errorf("save project config: %w", err)
			}
			output.Success("linked to project " + projectName)
			return nil
		},
	}
}

// xquare unlink
func newUnlinkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unlink",
		Short: "Remove project link from current directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			pc, _ := config.LoadProject()
			if pc == nil || pc.Project == "" {
				output.Info("no project linked in this directory")
				return nil
			}
			prev := pc.Project
			if err := config.SaveProject(&config.ProjectConfig{}); err != nil {
				return fmt.Errorf("remove project link: %w", err)
			}
			output.Success("unlinked from project " + prev)
			return nil
		},
	}
}

// xquare whoami
func newWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show current logged-in user",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadGlobal()
			if err != nil {
				return err
			}
			if cfg.Token == "" {
				output.Err("not logged in", "", "xquare login", "authenticate with GitHub")
				os.Exit(3)
			}
			project := ""
			if pc, _ := config.LoadProject(); pc != nil {
				project = pc.Project
			}
			// Fetch server-verified identity and token expiry from /auth/me
			expiresAt := ""
			username := cfg.Username
			if cfg.ServerURL != "" {
				c := api.New(cfg.ServerURL, cfg.Token)
				if me, err := c.GetMe(cmd.Context()); err == nil {
					if me.Username != "" {
						username = me.Username
					}
					expiresAt = me.ExpiresAt
				}
			}
			if isJSON, _ := cmd.Root().PersistentFlags().GetBool("json"); isJSON {
				m := map[string]any{"username": username}
				if project != "" {
					m["project"] = project
				}
				if expiresAt != "" {
					m["expires_at"] = expiresAt
				}
				return output.JSON(m)
			}
			fmt.Println(username)
			if project != "" {
				output.Info(fmt.Sprintf("project: %s", project))
			}
			if expiresAt != "" {
				if t, err := time.Parse("2006-01-02T15:04:05Z", expiresAt); err == nil {
					remaining := time.Until(t).Round(time.Minute)
					if remaining > 0 {
						output.Info(fmt.Sprintf("token expires in %s", remaining))
					} else {
						output.Warn("token expired — run: xquare login")
					}
				}
			}
			return nil
		},
	}
}
