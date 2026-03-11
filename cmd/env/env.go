package env

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/team-xquare/xquare-cli/internal/api"
	"github.com/team-xquare/xquare-cli/internal/output"
)

func NewEnvCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Manage application environment variables",
	}
	cmd.AddCommand(
		newEnvGetCmd(),
		newEnvSetCmd(),
		newEnvDeleteCmd(),
	)
	return cmd
}

func newEnvGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "get <app>",
		Short:   "Show environment variables",
		Aliases: []string{"list", "ls"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := api.FromCmd(cmd)
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			envs, err := c.GetEnv(cmd.Context(), project, args[0])
			if err != nil {
				return err
			}
			if api.IsJSON(cmd) {
				return output.JSON(envs)
			}
			if len(envs) == 0 {
				output.Info("no environment variables set")
				return nil
			}
			rows := make([][]string, 0, len(envs))
			for k, v := range envs {
				rows = append(rows, []string{k, v})
			}
			output.Table([]string{"KEY", "VALUE"}, rows)
			return nil
		},
	}
}

// xquare env set <app> KEY=VALUE [KEY=VALUE ...]
// xquare env set <app> --patch  (merge instead of full replace)
func newEnvSetCmd() *cobra.Command {
	var patch bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "set <app> KEY=VALUE ...",
		Short: "Set environment variables (full replace by default, --patch to merge)",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			appName := args[0]
			envs := make(map[string]string)
			for _, kv := range args[1:] {
				parts := strings.SplitN(kv, "=", 2)
				if len(parts) != 2 {
					return fmt.Errorf("invalid KEY=VALUE format: %s", kv)
				}
				envs[parts[0]] = parts[1]
			}

			if dryRun {
				output.Info(fmt.Sprintf("[dry-run] would set %d env var(s) on %s/%s", len(envs), project, appName))
				return nil
			}

			c := api.FromCmd(cmd)
			var result map[string]string
			if patch {
				result, err = c.PatchEnv(cmd.Context(), project, appName, envs)
			} else {
				result, err = c.SetEnv(cmd.Context(), project, appName, envs)
			}
			if err != nil {
				return err
			}
			if api.IsJSON(cmd) {
				return output.JSON(result)
			}
			output.Success(fmt.Sprintf("updated %d env var(s)", len(envs)))
			return nil
		},
	}
	cmd.Flags().BoolVar(&patch, "patch", false, "merge with existing vars instead of full replace")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would happen")
	return cmd
}

func newEnvDeleteCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "delete <app> <KEY>",
		Short: "Delete an environment variable",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			if dryRun {
				output.Info(fmt.Sprintf("[dry-run] would delete env var %s from %s/%s", args[1], project, args[0]))
				return nil
			}
			c := api.FromCmd(cmd)
			if err := c.DeleteEnvKey(cmd.Context(), project, args[0], args[1]); err != nil {
				return err
			}
			output.Success(fmt.Sprintf("deleted env var %s", args[1]))
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would happen")
	return cmd
}
