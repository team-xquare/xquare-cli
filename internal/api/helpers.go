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
// Priority: --server flag > XQUARE_SERVER_URL env > config file
// Priority: XQUARE_TOKEN env > config file token
func FromCmd(cmd *cobra.Command) *Client {
	cfg, err := config.LoadGlobal()
	if err != nil {
		output.Err("failed to load config", err.Error())
		os.Exit(6)
	}
	// --server flag takes highest precedence
	if cmd != nil {
		if s, _ := cmd.Root().PersistentFlags().GetString("server"); s != "" {
			cfg.ServerURL = s
		}
	}
	// XQUARE_TOKEN env var takes precedence over config file (for CI)
	if t := os.Getenv("XQUARE_TOKEN"); t != "" {
		cfg.Token = t
	}
	if cfg.Token == "" {
		output.Err("not logged in", "",
			"xquare login", "authenticate with GitHub",
			"XQUARE_TOKEN=<token> xquare ...", "use env var in CI")
		os.Exit(3)
	}
	return New(cfg.ServerURL, cfg.Token)
}

// IsJSON returns true if --json flag is set, or if --jq/--fields are set
// (filters imply JSON mode so they can operate on structured data).
func IsJSON(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("json")
	if !v {
		v, _ = cmd.Root().PersistentFlags().GetBool("json")
	}
	if !v {
		jq, _ := cmd.Root().PersistentFlags().GetString("jq")
		v = jq != ""
	}
	if !v {
		fields, _ := cmd.Root().PersistentFlags().GetStringSlice("fields")
		v = len(fields) > 0
	}
	return v
}

// RequireProject returns project name from flags, env var XQUARE_PROJECT, or .xquare/config.
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
	// XQUARE_PROJECT env var (for CI)
	if p = os.Getenv("XQUARE_PROJECT"); p != "" {
		return p, nil
	}
	// Try local project config
	pc, _ := config.LoadProject()
	if pc != nil && pc.Project != "" {
		return pc.Project, nil
	}
	return "", fmt.Errorf("project not specified\n\n  xquare link <project>              set default project\n  xquare ... --project <name>        use per-command\n  XQUARE_PROJECT=<name> xquare ...   use env var in CI")
}

// GetCurrentProject returns the currently linked project without requiring it.
// Returns empty string if not linked.
func GetCurrentProject(cmd *cobra.Command) (string, error) {
	p, _ := cmd.Root().PersistentFlags().GetString("project")
	if p != "" {
		return p, nil
	}
	if p = os.Getenv("XQUARE_PROJECT"); p != "" {
		return p, nil
	}
	pc, _ := config.LoadProject()
	if pc != nil {
		return pc.Project, nil
	}
	return "", nil
}

// JSONOut outputs v as JSON, applying --jq and --fields filters if set.
func JSONOut(cmd *cobra.Command, v any) error {
	jqExpr, _ := cmd.Root().PersistentFlags().GetString("jq")
	fields, _ := cmd.Root().PersistentFlags().GetStringSlice("fields")
	return output.JSONWithFilter(v, jqExpr, fields)
}
