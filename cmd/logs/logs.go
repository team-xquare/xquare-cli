package logs

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/spf13/cobra"

	"github.com/team-xquare/xquare-cli/internal/api"
	"github.com/team-xquare/xquare-cli/internal/output"
)

// ansiEscapeRe matches ANSI/VT100 escape sequences (CSI, OSC, etc.).
// Strips them before printing to prevent terminal injection attacks.
var ansiEscapeRe = regexp.MustCompile(`\x1b(?:[@-Z\\-_]|\[[0-9;]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\))`)

func stripANSI(s string) string { return ansiEscapeRe.ReplaceAllString(s, "") }

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
	// When not following, a single request is sufficient.
	if !follow {
		return streamRuntimeOnce(cmd, c, project, appName, tail, false, since)
	}

	// When following, reconnect on clean EOF so users see continuous output
	// even across pod restarts or transient server-side connection closes.
	maxRetries := 60 // ~2 min of retries at 2s intervals
	printed := false
	for attempt := 0; attempt <= maxRetries; attempt++ {
		select {
		case <-cmd.Context().Done():
			return nil
		default:
		}

		if attempt > 0 {
			if printed {
				output.Info("  connection lost, reconnecting...")
			} else {
				output.Info("  waiting for logs...")
			}
			time.Sleep(2 * time.Second)
		}

		err := streamRuntimeOnce(cmd, c, project, appName, tail, true, since)
		if err != nil {
			// Hard errors (app not found, not deployed, auth) — don't retry
			return err
		}
		// Clean EOF — keep retrying if --follow was requested
		printed = true
	}
	return nil
}

func streamRuntimeOnce(cmd *cobra.Command, c *api.Client, project, appName string, tail int64, follow bool, since string) error {
	resp, err := c.StreamLogs(cmd.Context(), project, appName, tail, follow, since)
	if err != nil {
		return fmt.Errorf("stream logs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
			Code  string `json:"code"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		switch e.Code {
		case "not_deployed":
			return fmt.Errorf("%s\n\n  xquare trigger %s --watch   # start deployment", e.Error, appName)
		case "start_timeout":
			return fmt.Errorf("%s\n\n  xquare app status %s          # check status\n  xquare logs %s --build         # check build logs", e.Error, appName, appName)
		default:
			if resp.StatusCode == 404 {
				return fmt.Errorf("app %q not found in project %q\n\n  xquare app list --project %s   # list apps in this project", appName, project, project)
			}
			if e.Error != "" {
				return fmt.Errorf("%s", e.Error)
			}
			return fmt.Errorf("failed to fetch logs (status %d) — please retry", resp.StatusCode)
		}
	}

	isJSON := api.IsJSON(cmd)
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 512*1024), 512*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if isJSON {
			_ = output.NDJSONLine(map[string]string{"line": line})
		} else {
			fmt.Println(stripANSI(line))
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
			return fmt.Errorf("no build history found\n\n  xquare trigger %s   # trigger first deployment", appName)
		}
		buildID = fmt.Sprintf("%v", builds[0]["id"])
		buildStatus := fmt.Sprintf("%v", builds[0]["status"])
		startedAt := fmt.Sprintf("%v", builds[0]["startedAt"])
		// Format startedAt to a local-friendly relative time if parseable
		startedStr := ""
		if startedAt != "" && startedAt != "<nil>" {
			if t, err := time.Parse(time.RFC3339, startedAt); err == nil {
				elapsed := time.Since(t).Round(time.Second)
				startedStr = fmt.Sprintf("  started %s ago", elapsed)
			}
		}
		output.Info(fmt.Sprintf("build: %s  [%s]%s", buildID, buildStatus, startedStr))
	}

	// Auto-reconnect loop for running/pending builds
	maxRetries := 30
	printed := false // tracks whether we've printed any log lines
	for attempt := 0; attempt <= maxRetries; attempt++ {
		select {
		case <-cmd.Context().Done():
			return nil
		default:
		}

		if attempt > 0 {
			// Check if build is still running before reconnecting
			builds, err := c.ListBuilds(cmd.Context(), project, appName)
			if err == nil {
				for _, b := range builds {
					if fmt.Sprintf("%v", b["id"]) == buildID {
						s := fmt.Sprintf("%v", b["status"])
						if s != "running" && s != "pending" {
							// Build finished — do one final fetch to get remaining logs
							if !printed {
								// First time seeing logs after build finished
								break
							}
							return nil
						}
					}
				}
			}
			if printed {
				output.Info("  connection lost, reconnecting...")
			} else {
				output.Info("  waiting for build logs...")
			}
			time.Sleep(2 * time.Second)
		}

		resp, err := c.StreamBuildLogs(cmd.Context(), project, appName, buildID, follow)
		if err != nil {
			if attempt == maxRetries {
				return fmt.Errorf("stream build logs: %w", err)
			}
			continue
		}

		if resp.StatusCode == http.StatusAccepted {
			// Build is still initializing (container not started yet) — treat like a retry.
			var body struct {
				Message string `json:"message"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&body)
			resp.Body.Close()
			if attempt == maxRetries {
				return fmt.Errorf("build initializing — logs not available yet\n\n  xquare app status %s   # check build status", appName)
			}
			if !printed {
				output.Info("  waiting for build to start...")
			}
			time.Sleep(3 * time.Second)
			continue
		}
		if resp.StatusCode >= 400 {
			var e struct {
				Error string `json:"error"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&e)
			resp.Body.Close()
			if attempt == maxRetries {
				if e.Error != "" {
					return fmt.Errorf("%s", e.Error)
				}
				return fmt.Errorf("server error: %d", resp.StatusCode)
			}
			continue
		}

		isJSON := api.IsJSON(cmd)
		// Use a larger scanner buffer for long Docker build lines
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 512*1024), 512*1024)
		for scanner.Scan() {
			line := scanner.Text()
			printed = true
			if isJSON {
				_ = output.NDJSONLine(map[string]string{"line": line})
			} else {
				fmt.Println(line)
			}
			select {
			case <-cmd.Context().Done():
				resp.Body.Close()
				return nil
			default:
			}
		}
		resp.Body.Close()

		// If not following, don't reconnect
		if !follow {
			return scanner.Err()
		}
		// Scanner ended (cleanly or with error) while following.
		// Check if the build has completed — if so, we're done; otherwise reconnect.
		if scanner.Err() == nil {
			builds, err := c.ListBuilds(cmd.Context(), project, appName)
			if err == nil {
				for _, b := range builds {
					if fmt.Sprintf("%v", b["id"]) == buildID {
						s := fmt.Sprintf("%v", b["status"])
						if s != "running" && s != "pending" {
							return nil // build finished, stop following
						}
						break
					}
				}
			}
			// Build still running or status unknown — reconnect
		}
	}
	return nil
}
