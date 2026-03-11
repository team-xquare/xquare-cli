package api

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/team-xquare/xquare-cli/internal/config"
	"github.com/team-xquare/xquare-cli/internal/output"
)

// FromCmd loads config and returns an authenticated client.
// Exits with code 3 if not logged in.
func FromCmd(_ *cobra.Command) *Client {
	cfg, err := config.LoadGlobal()
	if err != nil {
		output.Err("failed to load config", err.Error())
		os.Exit(6)
	}
	if cfg.Token == "" {
		output.Err("not logged in", "",
			"xquare login", "authenticate with GitHub")
		os.Exit(3)
	}
	return New(cfg.ServerURL, cfg.Token)
}

// IsJSON returns true if --json flag is set on the command
func IsJSON(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("json")
	if !v {
		v, _ = cmd.Root().PersistentFlags().GetBool("json")
	}
	return v
}

// RequireProject returns project name from --project flag or .xquare/config or error.
func RequireProject(cmd *cobra.Command) (string, error) {
	p, _ := cmd.Flags().GetString("project")
	if p != "" {
		return p, nil
	}
	// Try root's persistent --project flag
	p, _ = cmd.Root().PersistentFlags().GetString("project")
	if p != "" {
		return p, nil
	}
	// Try local project config
	pc, _ := config.LoadProject()
	if pc != nil && pc.Project != "" {
		return pc.Project, nil
	}
	return "", fmt.Errorf("project not specified (use --project or run 'xquare link <project>')")
}

// JSONOut outputs v as JSON, applying --jq and --fields filters if set.
func JSONOut(cmd *cobra.Command, v any) error {
	jqExpr, _ := cmd.Root().PersistentFlags().GetString("jq")
	fields, _ := cmd.Root().PersistentFlags().GetStringSlice("fields")
	return output.JSONWithFilter(v, jqExpr, fields)
}
