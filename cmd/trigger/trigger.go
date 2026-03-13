package trigger

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/team-xquare/xquare-cli/internal/api"
	"github.com/team-xquare/xquare-cli/internal/output"
)

func NewTriggerCmd() *cobra.Command {
	var watch bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "trigger <app>",
		Short: "Force re-run CI/CD (normally triggered automatically on git push)",
		Long: `Force a CI/CD run for the latest commit.

NOTE: CI/CD runs automatically when you push to GitHub — you do NOT need this command in normal workflow.
Use trigger only when:
  - The automatic webhook failed (network issue, GitHub App misconfiguration)
  - You need to re-deploy without making a code change
  - You want to watch deployment progress with --watch`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			appName := args[0]

			if dryRun {
				output.Info(fmt.Sprintf("[dry-run] would trigger %s/%s", project, appName))
				return nil
			}

			c := api.FromCmd(cmd)
			result, err := c.TriggerApp(cmd.Context(), project, appName)
			if err != nil {
				return err
			}

			buildID := fmt.Sprintf("%v", result["build"])
			output.Success(fmt.Sprintf("build started: %s/%s  [%s]", project, appName, buildID))

			if watch {
				output.Info("building and deploying... (Ctrl+C to stop)")
				return watchFull(cmd, c, project, appName, buildID)
			}
			output.Info(fmt.Sprintf("  xquare logs %s --build          # watch build logs", appName))
			output.Info(fmt.Sprintf("  xquare trigger %s --watch        # wait until deployed", appName))
			return nil
		},
	}

	cmd.Flags().BoolVarP(&watch, "watch", "w", false, "watch until deployment is fully running")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would happen")
	return cmd
}

// watchFull tracks: Phase 1 build → Phase 2 ArgoCD sync → Phase 3 running
func watchFull(cmd *cobra.Command, c *api.Client, project, app, buildID string) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	timeout := time.After(15 * time.Minute)

	phase := "building"
	lastMsg := ""
	failCount := 0

	printOnce := func(msg string) {
		if msg != lastMsg {
			output.Info(msg)
			lastMsg = msg
		}
	}

	for {
		select {
		case <-cmd.Context().Done():
			return nil
		case <-timeout:
			return fmt.Errorf("timeout (15min)\n\n  xquare logs %s --build   # check build logs", app)
		case <-ticker.C:
			switch phase {

			case "building":
				builds, err := c.ListBuilds(cmd.Context(), project, app)
				if err != nil {
					continue
				}
				var buildStatus string
				for _, b := range builds {
					if fmt.Sprintf("%v", b["id"]) == buildID {
						buildStatus = fmt.Sprintf("%v", b["status"])
						break
					}
				}
				switch buildStatus {
				case "success":
					output.Success("[1/3] build complete")
					output.Info("  [2/3] waiting for ArgoCD sync...")
					phase = "syncing"
				case "failed":
					return fmt.Errorf("build failed\n\n  xquare logs %s --build --build-id %s", app, buildID)
				default:
					printOnce(fmt.Sprintf("  [1/3] building...  [%s]", buildID))
				}

			case "syncing":
				status, err := c.GetAppStatus(cmd.Context(), project, app)
				if err != nil {
					continue
				}
				if fmt.Sprintf("%v", status["status"]) != "not_deployed" {
					phase = "deploying"
				}

			case "deploying":
				status, err := c.GetAppStatus(cmd.Context(), project, app)
				if err != nil {
					continue
				}
				s := fmt.Sprintf("%v", status["status"])
				running, desired := "?", "?"
				if sc, ok := status["scale"].(map[string]any); ok {
					running = fmt.Sprintf("%v", sc["running"])
					desired = fmt.Sprintf("%v", sc["desired"])
				}
				printOnce(fmt.Sprintf("  [3/3] deploying...  (%s/%s running)", running, desired))
				switch s {
				case "running":
					output.Success(fmt.Sprintf("deployed  (%s/%s running)", running, desired))
					return nil
				case "failed":
					failCount++
					if failCount >= 6 {
						return fmt.Errorf("deploy failed — pod did not start\n\n  xquare logs %s", app)
					}
					continue
				default:
					failCount = 0
				}
			}
		}
	}
}
