package auth

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/team-xquare/xquare-cli/internal/config"
	"github.com/team-xquare/xquare-cli/internal/output"
)

func NewAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication",
	}
	cmd.AddCommand(
		NewLoginCmd(),
		newLogoutCmd(),
		newAuthStatusCmd(),
	)
	return cmd
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Log out and remove stored credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadGlobal()
			if err != nil || cfg.Token == "" {
				output.Info("not logged in")
				return nil
			}
			username := cfg.Username
			cfg.Token = ""
			cfg.Username = ""
			if err := config.SaveGlobal(cfg); err != nil {
				return fmt.Errorf("remove credentials: %w", err)
			}
			if username != "" {
				output.Success(fmt.Sprintf("logged out (%s)", username))
			} else {
				output.Success("logged out")
			}
			return nil
		},
	}
}

func newAuthStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current authentication status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := config.LoadGlobal()
			token := ""
			if cfg != nil {
				token = cfg.Token
			}
			// XQUARE_TOKEN env var takes precedence
			if t := os.Getenv("XQUARE_TOKEN"); t != "" {
				token = t
			}
			if token == "" {
				if isJSON, _ := cmd.Root().PersistentFlags().GetBool("json"); isJSON {
					return output.JSON(map[string]any{"logged_in": false})
				}
				output.Info("not logged in")
				fmt.Fprintln(os.Stderr, "\n  xquare login   authenticate with GitHub")
				os.Exit(3)
			}
			username := ""
			if cfg != nil {
				username = cfg.Username
			}
			via := "config file"
			if os.Getenv("XQUARE_TOKEN") != "" {
				via = "XQUARE_TOKEN env var"
			}
			if isJSON, _ := cmd.Root().PersistentFlags().GetBool("json"); isJSON {
				return output.JSON(map[string]any{
					"logged_in": true,
					"username":  username,
					"via":       via,
					"server":    cfg.ServerURL,
				})
			}
			fmt.Printf("logged in as %s (via %s)\n", username, via)
			fmt.Printf("server: %s\n", cfg.ServerURL)
			return nil
		},
	}
}
