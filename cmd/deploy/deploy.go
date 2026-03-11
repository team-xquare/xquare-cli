package deploy

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/team-xquare/xquare-cli/internal/api"
	"github.com/team-xquare/xquare-cli/internal/output"
)

func NewDeployCmd() *cobra.Command {
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

			c := api.FromCmd(cmd)
			result, err := c.RedeployApp(cmd.Context(), project, appName)
			if err != nil {
				return err
			}

			buildID := fmt.Sprintf("%v", result["build"])
			output.Success(fmt.Sprintf("build started: %s/%s  [%s]", project, appName, buildID))

			if watch {
				output.Info("building and deploying... (Ctrl+C to stop)")
				return watchFull(cmd, c, project, appName, buildID)
			}
			output.Info(fmt.Sprintf("  xquare logs %s --build          # 빌드 로그 실시간 확인", appName))
			output.Info(fmt.Sprintf("  xquare deploy %s --watch        # 배포 완료까지 대기", appName))
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
			return fmt.Errorf("timeout (15min)\n\n  xquare logs %s --build   # 빌드 로그 확인", app)
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
					output.Success("[1/3] 빌드 완료")
					output.Info("  [2/3] ArgoCD 배포 동기화 중...")
					phase = "syncing"
				case "failed":
					return fmt.Errorf("빌드 실패\n\n  xquare logs %s --build --build-id %s", app, buildID)
				default:
					printOnce(fmt.Sprintf("  [1/3] 빌드 중...  [%s]", buildID))
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
				printOnce(fmt.Sprintf("  [3/3] 배포 중...  (%s/%s 실행 중)", running, desired))
				switch s {
				case "running":
					output.Success(fmt.Sprintf("배포 완료  (%s/%s running)", running, desired))
					return nil
				case "failed":
					return fmt.Errorf("배포 실패 — Pod가 시작되지 않음\n\n  xquare logs %s", app)
				}
			}
		}
	}
}
