package deploy

import (
	"fmt"
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

			if dryRun {
				output.Info(fmt.Sprintf("[dry-run] would redeploy %s/%s", project, appName))
				return nil
			}
			if githubToken == "" {
				return fmt.Errorf("--github-token required (your GitHub personal access token)")
			}

			c := api.FromCmd(cmd)
			result, err := c.RedeployApp(cmd.Context(), project, appName, githubToken)
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
				output.Info("watching deployment status...")
				return watchDeploy(cmd, c, project, appName)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&githubToken, "github-token", "", "GitHub personal access token")
	cmd.Flags().BoolVar(&watch, "watch", false, "watch deployment until ready")
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
			return fmt.Errorf("timeout waiting for deployment")
		case <-ticker.C:
			status, err := c.GetAppStatus(cmd.Context(), project, app)
			if err != nil {
				output.Info(fmt.Sprintf("  status check failed: %v", err))
				continue
			}
			s := fmt.Sprintf("%v", status["status"])
			output.Info(fmt.Sprintf("  → %s", s))
			if s == "Running" {
				output.Success("deployment complete")
				return nil
			}
			if s == "Failed" {
				return fmt.Errorf("deployment failed")
			}
		}
	}
}
