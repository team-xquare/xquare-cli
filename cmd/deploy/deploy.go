package deploy

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/team-xquare/xquare-cli/internal/api"
	"github.com/team-xquare/xquare-cli/internal/output"
)

func NewDeployCmd() *cobra.Command {
	var githubToken string
	var watch bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "deploy <app>",
		Short: "Trigger a re-deploy with the latest commit",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			appName := args[0]

			// Optional user token — if omitted, server uses GitHub App installation token
			token := githubToken
			if token == "" {
				token = os.Getenv("XQUARE_GITHUB_TOKEN")
			}

			if dryRun {
				output.Info(fmt.Sprintf("[dry-run] would redeploy %s/%s", project, appName))
				return nil
			}

			c := api.FromCmd(cmd)
			result, err := c.RedeployApp(cmd.Context(), project, appName, token)
			if err != nil {
				return err
			}

			sha := fmt.Sprintf("%v", result["sha"])
			short := sha
			if len(short) > 8 {
				short = short[:8]
			}
			output.Success(fmt.Sprintf("redeploy triggered: %s/%s @ %s", project, appName, short))

			if watch {
				output.Info("watching deployment status (Ctrl+C to stop)...")
				return watchDeploy(cmd, c, project, appName)
			}
			output.Info(fmt.Sprintf("  run: xquare app status %s   to check progress", appName))
			output.Info(fmt.Sprintf("  run: xquare logs %s          to stream logs", appName))
			return nil
		},
	}

	cmd.Flags().StringVar(&githubToken, "github-token", "", "GitHub personal access token (optional; defaults to GitHub App)")
	cmd.Flags().BoolVarP(&watch, "watch", "w", false, "watch deployment progress")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would happen")
	return cmd
}

func watchDeploy(cmd *cobra.Command, c *api.Client, project, app string) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	timeout := time.After(10 * time.Minute)

	for {
		select {
		case <-cmd.Context().Done():
			return nil
		case <-timeout:
			return fmt.Errorf("timeout waiting for deployment (10min)")
		case <-ticker.C:
			status, err := c.GetAppStatus(cmd.Context(), project, app)
			if err != nil {
				output.Info(fmt.Sprintf("  status check failed: %v", err))
				continue
			}
			s := fmt.Sprintf("%v", status["status"])
			running, desired := "?", "?"
			if sc, ok := status["scale"].(map[string]any); ok {
				running = fmt.Sprintf("%v", sc["running"])
				desired = fmt.Sprintf("%v", sc["desired"])
			}
			output.Info(fmt.Sprintf("  → %s  (%s/%s running)", s, running, desired))
			if s == "running" {
				output.Success("deployment complete")
				return nil
			}
			if s == "failed" {
				return fmt.Errorf("deployment failed — run: xquare logs %s", app)
			}
		}
	}
}
