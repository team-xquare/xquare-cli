package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/team-xquare/xquare-cli/cmd/addon"
	"github.com/team-xquare/xquare-cli/cmd/app"
	"github.com/team-xquare/xquare-cli/cmd/auth"
	"github.com/team-xquare/xquare-cli/cmd/deploy"
	"github.com/team-xquare/xquare-cli/cmd/env"
	"github.com/team-xquare/xquare-cli/cmd/logs"
	"github.com/team-xquare/xquare-cli/cmd/mcp"
	"github.com/team-xquare/xquare-cli/cmd/project"
	"github.com/team-xquare/xquare-cli/internal/api"
	"github.com/team-xquare/xquare-cli/internal/config"
	"github.com/team-xquare/xquare-cli/internal/output"
)

var rootCmd = &cobra.Command{
	Use:   "xquare",
	Short: "xquare PaaS CLI — manage your projects, apps, and services",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Suppress usage on runtime errors (usage only makes sense for wrong flags/args)
		cmd.SilenceUsage = true
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
		mcp.NewMCPCmd(),
		newLinkCmd(),
		newUnlinkCmd(),
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
			if isJSON, _ := cmd.Root().PersistentFlags().GetBool("json"); isJSON {
				m := map[string]string{"username": cfg.Username}
				if project != "" {
					m["project"] = project
				}
				return output.JSON(m)
			}
			fmt.Println(cfg.Username)
			if project != "" {
				output.Info(fmt.Sprintf("project: %s", project))
			}
			return nil
		},
	}
}
