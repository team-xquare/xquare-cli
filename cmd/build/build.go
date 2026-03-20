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
			// Request the limit from the server to avoid fetching more than needed.
			fetchLimit := limit
			if fetchLimit <= 0 {
				fetchLimit = 50 // 0 = all, let server return max
			}
			builds, err := c.ListBuildsLimit(cmd.Context(), project, appName, fetchLimit)
			if err != nil {
				return err
			}
			if api.IsJSON(cmd) {
				return output.JSON(builds)
			}
			if len(builds) == 0 {
				output.Info(fmt.Sprintf("no builds found for %s/%s", project, appName))
				output.Info(fmt.Sprintf("  xquare trigger %s   # trigger first build", appName))
				return nil
			}
			rows := make([][]string, 0, len(builds))
			for _, b := range builds {
				id := fmt.Sprintf("%v", b["id"])
				status := fmt.Sprintf("%v", b["status"])
				startedAt := ""
				if s := fmt.Sprintf("%v", b["startedAt"]); s != "" && s != "<nil>" {
					if t, err := time.Parse(time.RFC3339, s); err == nil {
						startedAt = t.Local().Format("2006-01-02 15:04:05") + fmt.Sprintf("  (%s ago)", time.Since(t).Round(time.Second))
					} else {
						startedAt = s
					}
				}
				rows = append(rows, []string{id, status, startedAt})
			}
			output.Table([]string{"BUILD ID", "STATUS", "STARTED"}, rows)
			output.Info(fmt.Sprintf("\n  xquare logs %s --build --follow   # stream latest build logs", appName))
			return nil
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "l", 10, "maximum number of builds to show (0 = all, max 50)")
	return cmd
}
