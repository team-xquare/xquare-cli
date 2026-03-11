package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/team-xquare/xquare-cli/cmd/addon"
	"github.com/team-xquare/xquare-cli/cmd/app"
	"github.com/team-xquare/xquare-cli/cmd/auth"
	"github.com/team-xquare/xquare-cli/cmd/db"
	"github.com/team-xquare/xquare-cli/cmd/deploy"
	"github.com/team-xquare/xquare-cli/cmd/env"
	"github.com/team-xquare/xquare-cli/cmd/logs"
	"github.com/team-xquare/xquare-cli/cmd/mcp"
	"github.com/team-xquare/xquare-cli/cmd/project"
	"github.com/team-xquare/xquare-cli/internal/config"
	"github.com/team-xquare/xquare-cli/internal/output"
)

var rootCmd = &cobra.Command{
	Use:   "xquare",
	Short: "xquare PaaS CLI — manage your projects, apps, and services",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		jq, _ := cmd.Root().PersistentFlags().GetString("jq")
		fields, _ := cmd.Root().PersistentFlags().GetStringSlice("fields")
		output.SetGlobalFilters(jq, fields)
	},
}

func init() {
	rootCmd.PersistentFlags().Bool("json", false, "output as JSON")
	rootCmd.PersistentFlags().String("jq", "", "filter JSON output with a jq expression")
	rootCmd.PersistentFlags().StringSlice("fields", nil, "select fields from JSON response (e.g. name,status)")
	rootCmd.PersistentFlags().StringP("project", "p", "", "project name (overrides .xquare/config)")

	rootCmd.AddCommand(
		auth.NewLoginCmd(),
		project.NewProjectCmd(),
		app.NewAppCmd(),
		deploy.NewDeployCmd(),
		env.NewEnvCmd(),
		addon.NewAddonCmd(),
		logs.NewLogsCmd(),
		db.NewDBCmd(),
		mcp.NewMCPCmd(),
		newLinkCmd(),
		newWhoamiCmd(),
	)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// xquare link <project>
func newLinkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "link <project>",
		Short: "Link current directory to a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.SaveProject(&config.ProjectConfig{Project: args[0]}); err != nil {
				return fmt.Errorf("save project config: %w", err)
			}
			output.Success("linked to project " + args[0])
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
			if isJSON, _ := cmd.Root().PersistentFlags().GetBool("json"); isJSON {
				return output.JSON(map[string]string{"username": cfg.Username})
			}
			fmt.Println(cfg.Username)
			return nil
		},
	}
}
