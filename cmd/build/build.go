package build

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/team-xquare/xquare-cli/internal/api"
	"github.com/team-xquare/xquare-cli/internal/output"
)

func NewBuildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Manage CI/CD builds",
	}
	cmd.AddCommand(newBuildListCmd())
	return cmd
}

func newBuildListCmd() *cobra.Command {
	var limit int
	var statusFilter string
	cmd := &cobra.Command{
		Use:     "list <app>",
		Short:   "List recent CI/CD builds for an app",
		Aliases: []string{"ls"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := api.FromCmd(cmd)
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			appName := args[0]
			// When a status filter is active, always fetch the maximum so --limit
			// applies to the filtered result, not the pre-filtered fetch.
			// Without this, "--status=success --limit=3" might fetch only 3 builds,
			// filter them, and return 0-3 instead of up to 3 success builds.
			fetchLimit := limit
			if statusFilter != "" {
				fetchLimit = 50 // fetch all, apply --limit after filtering
			} else if fetchLimit <= 0 {
				fetchLimit = 50 // 0 = all, let server return max
			}
			builds, err := c.ListBuildsLimit(cmd.Context(), project, appName, fetchLimit)
			if err != nil {
				return err
			}
			if statusFilter != "" {
				filtered := make([]map[string]any, 0)
				for _, b := range builds {
					if fmt.Sprintf("%v", b["status"]) == statusFilter {
						filtered = append(filtered, b)
					}
				}
				builds = filtered
			}
			// Apply --limit after filtering so it counts matching results, not total fetched.
			if limit > 0 && len(builds) > limit {
				builds = builds[:limit]
			}
			if api.IsJSON(cmd) {
				// Enrich each build with a computed "duration" field so callers
				// don't have to subtract startedAt from finishedAt themselves.
				enriched := make([]map[string]any, 0, len(builds))
				for _, b := range builds {
					entry := make(map[string]any, len(b)+1)
					for k, v := range b {
						entry[k] = v
					}
					startStr := fmt.Sprintf("%v", b["startedAt"])
					finStr := fmt.Sprintf("%v", b["finishedAt"])
					if startStr != "" && startStr != "<nil>" && finStr != "" && finStr != "<nil>" {
						if st, err1 := time.Parse(time.RFC3339, startStr); err1 == nil {
							if ft, err2 := time.Parse(time.RFC3339, finStr); err2 == nil {
								entry["duration"] = ft.Sub(st).Round(time.Second).String()
							}
						}
					}
					enriched = append(enriched, entry)
				}
				return output.JSON(enriched)
			}
			if len(builds) == 0 {
				if statusFilter != "" {
					output.Info(fmt.Sprintf("no %s builds found for %s/%s", statusFilter, project, appName))
				} else {
					output.Info(fmt.Sprintf("no builds found for %s/%s", project, appName))
					output.Info(fmt.Sprintf("  xquare trigger %s   # trigger first build", appName))
				}
				return nil
			}
			rows := make([][]string, 0, len(builds))
			for _, b := range builds {
				id := fmt.Sprintf("%v", b["id"])
				status := fmt.Sprintf("%v", b["status"])
				startedAt := ""
				var startTime time.Time
				if s := fmt.Sprintf("%v", b["startedAt"]); s != "" && s != "<nil>" {
					if t, err := time.Parse(time.RFC3339, s); err == nil {
						startTime = t
						startedAt = t.Local().Format("2006-01-02 15:04:05") + fmt.Sprintf("  (%s ago)", time.Since(t).Round(time.Second))
					} else {
						startedAt = s
					}
				}
				duration := ""
				if finStr := fmt.Sprintf("%v", b["finishedAt"]); finStr != "" && finStr != "<nil>" {
					if ft, err := time.Parse(time.RFC3339, finStr); err == nil {
						if !startTime.IsZero() {
							duration = ft.Sub(startTime).Round(time.Second).String()
						}
					}
				} else if !startTime.IsZero() && (status == "running" || status == "pending") {
					// For in-progress builds, show elapsed time
					duration = time.Since(startTime).Round(time.Second).String() + " (running)"
				}
				rows = append(rows, []string{id, status, startedAt, duration})
				if status == "failed" {
					if msg := fmt.Sprintf("%v", b["message"]); msg != "" && msg != "<nil>" {
						rows = append(rows, []string{"  └ " + msg, "", "", ""})
					}
				}
			}
			output.Table([]string{"BUILD ID", "STATUS", "STARTED", "DURATION"}, rows)
			output.Info(fmt.Sprintf("\n  xquare logs %s --build --follow   # stream latest build logs", appName))
			return nil
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "l", 10, "maximum number of builds to show (0 = all, max 50)")
	cmd.Flags().StringVar(&statusFilter, "status", "", "filter by status: running, success, failed, pending")
	return cmd
}
