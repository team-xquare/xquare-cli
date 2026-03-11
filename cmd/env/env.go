package env

import (
	"bufio"
	"fmt"
	"os"
	"sort"
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
		newEnvPullCmd(),
		newEnvPushCmd(),
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
			// Sort for stable output
			keys := make([]string, 0, len(envs))
			for k := range envs {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			rows := make([][]string, 0, len(envs))
			for _, k := range keys {
				rows = append(rows, []string{k, envs[k]})
			}
			output.Table([]string{"KEY", "VALUE"}, rows)
			return nil
		},
	}
}

// set: default is MERGE (patch). Use --replace for full replace.
func newEnvSetCmd() *cobra.Command {
	var replace bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "set <app> KEY=VALUE ...",
		Short: "Set environment variables (merges by default; use --replace for full replace)",
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
				key := parts[0]
				if key == "" {
					return fmt.Errorf("invalid KEY=VALUE format: key cannot be empty")
				}
				if strings.ContainsAny(key, " \t") {
					return fmt.Errorf("invalid key %q: keys cannot contain spaces or tabs", key)
				}
				envs[key] = parts[1]
			}

			if dryRun {
				action := "merge"
				if replace {
					action = "replace"
				}
				output.Info(fmt.Sprintf("[dry-run] would %s %d env var(s) on %s/%s", action, len(envs), project, appName))
				for k, v := range envs {
					output.Info(fmt.Sprintf("  %s=%s", k, v))
				}
				return nil
			}

			c := api.FromCmd(cmd)
			var result map[string]string
			if replace {
				result, err = c.SetEnv(cmd.Context(), project, appName, envs)
			} else {
				result, err = c.PatchEnv(cmd.Context(), project, appName, envs)
			}
			if err != nil {
				return err
			}
			if api.IsJSON(cmd) {
				return output.JSON(result)
			}
			output.Success(fmt.Sprintf("set %d env var(s) on %s/%s", len(envs), project, appName))
			return nil
		},
	}
	cmd.Flags().BoolVar(&replace, "replace", false, "full replace instead of merge (DANGER: deletes all existing vars)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would happen")
	return cmd
}

func newEnvDeleteCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "delete <app> <KEY> [KEY...]",
		Short: "Delete one or more environment variables",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			appName := args[0]
			keys := args[1:]
			if dryRun {
				for _, k := range keys {
					output.Info(fmt.Sprintf("[dry-run] would delete env var %s from %s/%s", k, project, appName))
				}
				return nil
			}
			c := api.FromCmd(cmd)
			for _, k := range keys {
				if err := c.DeleteEnvKey(cmd.Context(), project, appName, k); err != nil {
					return err
				}
			}
			output.Success(fmt.Sprintf("deleted %d env var(s)", len(keys)))
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would happen")
	return cmd
}

// pull: saves env vars to a .env file
func newEnvPullCmd() *cobra.Command {
	var outputFile string
	cmd := &cobra.Command{
		Use:   "pull <app>",
		Short: "Pull env vars to a .env file",
		Args:  cobra.ExactArgs(1),
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

			// Sort keys
			keys := make([]string, 0, len(envs))
			for k := range envs {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			var lines []string
			for _, k := range keys {
				v := envs[k]
				// Quote value if contains spaces or special chars
				if strings.ContainsAny(v, " \t\n\"'\\") {
					v = fmt.Sprintf("%q", v)
				}
				lines = append(lines, fmt.Sprintf("%s=%s", k, v))
			}
			content := strings.Join(lines, "\n") + "\n"

			if outputFile == "" || outputFile == "-" {
				fmt.Print(content)
				return nil
			}
			if err := os.WriteFile(outputFile, []byte(content), 0600); err != nil {
				return fmt.Errorf("write %s: %w", outputFile, err)
			}
			output.Success(fmt.Sprintf("saved %d env var(s) to %s", len(envs), outputFile))
			return nil
		},
	}
	cmd.Flags().StringVarP(&outputFile, "output", "o", ".env", "output file (- for stdout)")
	return cmd
}

// push: loads env vars from a .env file and merges
func newEnvPushCmd() *cobra.Command {
	var inputFile string
	var replace bool
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "push <app>",
		Short: "Push env vars from a .env file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			appName := args[0]

			f, err := os.Open(inputFile)
			if err != nil {
				return err
			}
			defer f.Close()

			envs := make(map[string]string)
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				parts := strings.SplitN(line, "=", 2)
				if len(parts) != 2 {
					continue
				}
				k := strings.TrimSpace(parts[0])
				v := strings.TrimSpace(parts[1])
				// unquote if quoted
				if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
					v = strings.Trim(v, "\"")
				}
				envs[k] = v
			}
			if err := scanner.Err(); err != nil {
				return err
			}

			if dryRun {
				action := "merge"
				if replace {
					action = "replace"
				}
				output.Info(fmt.Sprintf("[dry-run] would %s %d env var(s) on %s/%s from %s", action, len(envs), project, appName, inputFile))
				for k, v := range envs {
					output.Info(fmt.Sprintf("  %s=%s", k, v))
				}
				return nil
			}

			c := api.FromCmd(cmd)
			var result map[string]string
			if replace {
				result, err = c.SetEnv(cmd.Context(), project, appName, envs)
			} else {
				result, err = c.PatchEnv(cmd.Context(), project, appName, envs)
			}
			if err != nil {
				return err
			}
			if api.IsJSON(cmd) {
				return output.JSON(result)
			}
			output.Success(fmt.Sprintf("pushed %d env var(s) to %s/%s", len(envs), project, appName))
			return nil
		},
	}
	cmd.Flags().StringVarP(&inputFile, "file", "f", ".env", "input .env file")
	cmd.Flags().BoolVar(&replace, "replace", false, "full replace instead of merge")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would happen")
	return cmd
}
