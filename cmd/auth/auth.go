package auth

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/team-xquare/xquare-cli/internal/api"
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
			serverURL := ""
			if cfg != nil {
				username = cfg.Username
				serverURL = cfg.ServerURL
			}
			via := "config file"
			if os.Getenv("XQUARE_TOKEN") != "" {
				via = "XQUARE_TOKEN env var"
			}
			// Verify the token is still valid by calling /auth/me
			tokenValid := true
			tokenExpired := false
			expiresAt := ""
			if serverURL != "" {
				c := api.New(serverURL, token)
				if me, err := c.GetMe(cmd.Context()); err != nil {
					if strings.Contains(err.Error(), "session expired") || strings.Contains(err.Error(), "token_expired") {
						tokenValid = false
						tokenExpired = true
					} else {
						// Network error or server unavailable — don't mark as invalid
						tokenValid = true
					}
				} else {
					if me.Username != "" {
						// Server confirmed identity; use server's username as authoritative
						username = me.Username
					}
					expiresAt = me.ExpiresAt
				}
			}
			if isJSON, _ := cmd.Root().PersistentFlags().GetBool("json"); isJSON {
				m := map[string]any{
					"logged_in": tokenValid,
					"username":  username,
					"via":       via,
					"server":    serverURL,
				}
				if tokenExpired {
					m["expired"] = true
				}
				if expiresAt != "" {
					m["expires_at"] = expiresAt
				}
				return output.JSON(m)
			}
			if !tokenValid {
				output.Warn("session expired — run: xquare login")
				os.Exit(3)
			}
			fmt.Printf("logged in as %s (via %s)\n", username, via)
			fmt.Printf("server: %s\n", serverURL)
			if expiresAt != "" {
				if t, err := time.Parse("2006-01-02T15:04:05Z", expiresAt); err == nil {
					remaining := time.Until(t).Round(time.Minute)
					if remaining > 0 {
						fmt.Printf("token expires: %s (in %s)\n", t.Local().Format("2006-01-02 15:04"), remaining)
					} else {
						fmt.Printf("token expires: %s (expired)\n", t.Local().Format("2006-01-02 15:04"))
					}
				}
			}
			return nil
		},
	}
}
