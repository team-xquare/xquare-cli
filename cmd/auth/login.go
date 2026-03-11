package auth

import (
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"github.com/team-xquare/xquare-cli/internal/api"
	"github.com/team-xquare/xquare-cli/internal/config"
	"github.com/team-xquare/xquare-cli/internal/output"
)

func NewLoginCmd() *cobra.Command {
	var serverURL string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with GitHub",
		Long: `Opens a local server to receive the GitHub OAuth callback.
Prints the URL to open in your browser.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := config.LoadGlobal()
			if cfg == nil {
				cfg = &config.GlobalConfig{}
			}
			if serverURL != "" {
				cfg.ServerURL = serverURL
			}

			code, err := receiveOAuthCode()
			if err != nil {
				return fmt.Errorf("oauth: %w", err)
			}

			apiClient := api.New(cfg.ServerURL, "")
			resp, err := apiClient.AuthGitHub(cmd.Context(), code)
			if err != nil {
				return fmt.Errorf("auth: %w", err)
			}

			cfg.Token = resp.Token
			cfg.Username = resp.Username
			if err := config.SaveGlobal(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			output.Success(fmt.Sprintf("logged in as %s", resp.Username))
			return nil
		},
	}

	cmd.Flags().StringVar(&serverURL, "server", "", "xquare server URL (overrides config)")
	return cmd
}

// receiveOAuthCode starts a local HTTP server on :9999, prints the GitHub OAuth URL,
// and waits for the callback to receive the code.
func receiveOAuthCode() (string, error) {
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	cfg, _ := config.LoadGlobal()
	clientID := getClientID(cfg.ServerURL)

	authURL := fmt.Sprintf(
		"https://github.com/login/oauth/authorize?client_id=%s&redirect_uri=%s&scope=repo",
		clientID, "http://localhost:9999/callback",
	)

	output.Info("Open this URL in your browser:")
	output.Info(authURL)

	srv := &http.Server{Addr: ":9999"}
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no code in callback")
			fmt.Fprintln(w, "Authentication failed. You can close this tab.")
			return
		}
		codeCh <- code
		fmt.Fprintln(w, "Authentication successful! You can close this tab.")
		go func() { _ = srv.Close() }()
	})

	_ = cfg // avoid unused
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case code := <-codeCh:
		return code, nil
	case err := <-errCh:
		return "", err
	}
}

func getClientID(serverURL string) string {
	if v := os.Getenv("XQUARE_GITHUB_CLIENT_ID"); v != "" {
		return v
	}
	// Default DSM on-prem client ID
	return "Iv23liNcqXa5XNLu40Zj"
}
