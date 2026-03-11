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

			// Token priority: --github-token flag > XQUARE_GITHUB_TOKEN env > error
			token := githubToken
			if token == "" {
				token = os.Getenv("XQUARE_GITHUB_TOKEN")
			}
			if token == "" {
				return fmt.Errorf("GitHub token required\n\n" +
					"  Set XQUARE_GITHUB_TOKEN env var, or pass --github-token <token>\n" +
					"  Token needs repo scope to read commit SHAs")
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

	cmd.Flags().StringVar(&githubToken, "github-token", "", "GitHub personal access token (or set XQUARE_GITHUB_TOKEN)")
	cmd.Flags().BoolVarP(&watch, "watch", "w", false, "watch deployment until Running or Failed")
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
			ready, desired := "?", "?"
			if rep, ok := status["replicas"].(map[string]any); ok {
				ready = fmt.Sprintf("%v", rep["ready"])
				desired = fmt.Sprintf("%v", rep["desired"])
			}
			output.Info(fmt.Sprintf("  → %s  (%s/%s ready)", s, ready, desired))
			if s == "Running" {
				output.Success("deployment complete")
				return nil
			}
			if s == "Failed" {
				return fmt.Errorf("deployment failed — run: xquare logs %s", app)
			}
		}
	}
}
