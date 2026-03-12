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

			resp, err := c.StreamLogs(cmd.Context(), project, appName, tail, follow)
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
		},
	}

	cmd.Flags().Int64VarP(&tail, "tail", "n", 100, "number of lines to show from end")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow log output")
	return cmd
}
