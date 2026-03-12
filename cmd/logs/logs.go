package logs

import (
	"bufio"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/team-xquare/xquare-cli/internal/api"
	"github.com/team-xquare/xquare-cli/internal/output"
)

func NewLogsCmd() *cobra.Command {
	var tail int64
	var follow bool
	var build bool
	var since string
	var buildID string

	cmd := &cobra.Command{
		Use:   "logs <app>",
		Short: "Stream application logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := api.FromCmd(cmd)
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			appName := args[0]

			if build {
				return streamBuildLogs(cmd, c, project, appName, buildID, follow)
			}
			return streamRuntimeLogs(cmd, c, project, appName, tail, follow, since)
		},
	}

	cmd.Flags().Int64VarP(&tail, "tail", "n", 100, "number of lines to show from end")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow log output")
	cmd.Flags().BoolVar(&build, "build", false, "show CI build logs instead of runtime logs")
	cmd.Flags().StringVar(&since, "since", "", "show logs since duration (e.g. 1h, 30m, 5m)")
	cmd.Flags().StringVar(&buildID, "build-id", "latest", "specific build ID (use with --build)")
	return cmd
}

func streamRuntimeLogs(cmd *cobra.Command, c *api.Client, project, appName string, tail int64, follow bool, since string) error {
	resp, err := c.StreamLogs(cmd.Context(), project, appName, tail, follow, since)
	if err != nil {
		return fmt.Errorf("stream logs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("server error: %d", resp.StatusCode)
	}

	isJSON := api.IsJSON(cmd)
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if isJSON {
			_ = output.NDJSONLine(map[string]string{"line": line})
		} else {
			fmt.Println(line)
		}
		select {
		case <-cmd.Context().Done():
			return nil
		default:
		}
	}
	return scanner.Err()
}

func streamBuildLogs(cmd *cobra.Command, c *api.Client, project, appName, buildID string, follow bool) error {
	if buildID == "latest" {
		builds, err := c.ListBuilds(cmd.Context(), project, appName)
		if err != nil {
			return fmt.Errorf("list builds: %w", err)
		}
		if len(builds) == 0 {
			return fmt.Errorf("no builds found for %s/%s\n\nPush code or run: xquare deploy %s", project, appName, appName)
		}
		buildID = fmt.Sprintf("%v", builds[0]["id"])
		buildStatus := fmt.Sprintf("%v", builds[0]["status"])
		output.Info(fmt.Sprintf("build: %s  [%s]", buildID, buildStatus))
	}

	resp, err := c.StreamBuildLogs(cmd.Context(), project, appName, buildID, follow)
	if err != nil {
		return fmt.Errorf("stream build logs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("server error: %d", resp.StatusCode)
	}

	isJSON := api.IsJSON(cmd)
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if isJSON {
			_ = output.NDJSONLine(map[string]string{"line": line})
		} else {
			fmt.Println(line)
		}
		select {
		case <-cmd.Context().Done():
			return nil
		default:
		}
	}
	return scanner.Err()
}
