package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/team-xquare/xquare-cli/cmd/schema"
	"github.com/team-xquare/xquare-cli/internal/api"
	"github.com/team-xquare/xquare-cli/internal/config"
	"github.com/team-xquare/xquare-cli/internal/output"
)

func NewMCPCmd() *cobra.Command {
	var installClaude, installCursor, installVSCode bool

	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Start MCP server (for AI agent integration)",
		Long: `Start xquare as an MCP (Model Context Protocol) server.

Use --claude, --cursor, or --vscode to register xquare as an MCP server
in your AI tool instead of starting the server directly.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Handle IDE registration flags
			if installClaude || installCursor || installVSCode {
				return registerMCP(installClaude, installCursor, installVSCode)
			}

			cfg, err := config.LoadGlobal()
			if err != nil || cfg.Token == "" {
				return fmt.Errorf("not logged in — run: xquare login")
			}
			client := api.New(cfg.ServerURL, cfg.Token)

			s := server.NewMCPServer("xquare", "1.0.0",
				server.WithToolCapabilities(true),
			)

			// ── Project tools ──────────────────────────────────────────────

			s.AddTool(mcp.NewTool("list_projects",
				mcp.WithDescription("List all projects you have access to. Returns array of project name strings."),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				data, err := client.ListProjects(ctx)
				return jsonResult(data, err)
			})

			s.AddTool(mcp.NewTool("get_project",
				mcp.WithDescription("Get project details including all apps and addons."),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				project, err := req.RequireString("project")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				data, err := client.GetProject(ctx, project)
				return jsonResult(data, err)
			})

			s.AddTool(mcp.NewTool("create_project",
				mcp.WithDescription("Create a new project. IMPORTANT: name must be lowercase letters and numbers only — NO hyphens allowed. Examples: myproject, dsm2025, backend01"),
				mcp.WithString("name", mcp.Required(), mcp.Description("Project name: ^[a-z0-9]{2,63}$ — lowercase letters and numbers only, no hyphens")),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				name, err := req.RequireString("name")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				if err := client.CreateProject(ctx, name); err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				return mcp.NewToolResultText(fmt.Sprintf("created project %q", name)), nil
			})

			s.AddTool(mcp.NewTool("delete_project",
				mcp.WithDescription("Delete a project and ALL its apps and addons. Irreversible."),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name to delete")),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				project, err := req.RequireString("project")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				if err := client.DeleteProject(ctx, project); err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				return mcp.NewToolResultText(fmt.Sprintf("deleted project %q", project)), nil
			})

			// ── App tools ──────────────────────────────────────────────────

			s.AddTool(mcp.NewTool("list_apps",
				mcp.WithDescription("List applications in a project."),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				project, err := req.RequireString("project")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				data, err := client.ListApps(ctx, project)
				return jsonResult(data, err)
			})

			s.AddTool(mcp.NewTool("get_app",
				mcp.WithDescription("Get application configuration (build type, endpoints, GitHub repo info)."),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
				mcp.WithString("app", mcp.Required(), mcp.Description("App name")),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				project, err := req.RequireString("project")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				app, err := req.RequireString("app")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				data, err := client.GetApp(ctx, project, app)
				return jsonResult(data, err)
			})

			s.AddTool(mcp.NewTool("get_app_status",
				mcp.WithDescription("Get deployment status: running/pending/failed/stopped/not_deployed, instance count, image version. If status is not_deployed, run deploy tool first."),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
				mcp.WithString("app", mcp.Required(), mcp.Description("App name")),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				project, err := req.RequireString("project")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				app, err := req.RequireString("app")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				data, err := client.GetAppStatus(ctx, project, app)
				return jsonResult(data, err)
			})

			s.AddTool(mcp.NewTool("create_app",
				mcp.WithDescription(`Create a new application.

CONSTRAINTS:
- app name: lowercase letters, numbers, hyphens only — ^[a-z0-9][a-z0-9-]{0,61}[a-z0-9]$
- build_type: gradle | nodejs | react | vite | vue | nextjs | nextjs-export | go | rust | maven | django | flask | docker
- endpoints format: ["8080"] or ["8080:api.dsmhs.kr"] or ["8080:api.dsmhs.kr,admin.dsmhs.kr"] (repeatable for multiple ports)
- GitHub App must be installed on the repo (error will include install URL if not)

After creation, CI pipeline takes ~2-3 minutes to prepare. Then call deploy tool.`),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
				mcp.WithString("app", mcp.Required(), mcp.Description("App name: lowercase, hyphens ok, 2-63 chars")),
				mcp.WithString("build_type", mcp.Required(), mcp.Description("gradle|nodejs|react|vite|vue|nextjs|nextjs-export|go|rust|maven|django|flask|docker")),
				mcp.WithString("github_owner", mcp.Required(), mcp.Description("GitHub org or user name")),
				mcp.WithString("github_repo", mcp.Required(), mcp.Description("GitHub repository name")),
				mcp.WithString("github_branch", mcp.Description("GitHub branch (default: main)")),
				mcp.WithString("endpoints", mcp.Description(`JSON array of endpoint strings. Examples: ["8080"] or ["8080:api.dsmhs.kr"] or ["8080:api.dsmhs.kr","9090"]`)),
				mcp.WithString("trigger_paths", mcp.Description("Comma-separated CI trigger paths, e.g. src/**,Dockerfile (optional)")),
				mcp.WithString("build_options", mcp.Description(`Optional JSON for build-type specific options. Examples:
- gradle: {"javaVersion":"17","buildCommand":"./gradlew bootJar","jarOutputPath":"/build/libs/*.jar"}
- nodejs:  {"nodeVersion":"20","buildCommand":"npm install","startCommand":"npm start"}
- go:      {"goVersion":"1.23","buildCommand":"go build -o app .","binaryName":"app"}
- docker:  {"dockerfilePath":"./Dockerfile","contextPath":"."}`)),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				project, err := req.RequireString("project")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				appName, err := req.RequireString("app")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				buildType, err := req.RequireString("build_type")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				owner, err := req.RequireString("github_owner")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				repoName, err := req.RequireString("github_repo")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				branch := req.GetString("github_branch", "main")

				body := map[string]any{
					"name": appName,
					"github": map[string]any{
						"owner":  owner,
						"repo":   repoName,
						"branch": branch,
					},
				}

				// Parse build options
				buildOpts := map[string]any{}
				if boStr := req.GetString("build_options", ""); boStr != "" {
					if e := json.Unmarshal([]byte(boStr), &buildOpts); e != nil {
						return mcp.NewToolResultError("invalid build_options JSON: " + e.Error()), nil
					}
				}
				body["build"] = map[string]any{buildType: buildOpts}

				// Parse endpoints
				if epStr := req.GetString("endpoints", ""); epStr != "" {
					var eps []string
					if e := json.Unmarshal([]byte(epStr), &eps); e != nil {
						return mcp.NewToolResultError("invalid endpoints JSON: " + e.Error()), nil
					}
					var endpoints []map[string]any
					for _, ep := range eps {
						parts := strings.SplitN(ep, ":", 2)
						port := 0
						if _, e := fmt.Sscanf(parts[0], "%d", &port); e != nil || port <= 0 {
							return mcp.NewToolResultError(fmt.Sprintf("invalid endpoint port in %q", ep)), nil
						}
						m := map[string]any{"port": port}
						if len(parts) == 2 && parts[1] != "" {
							m["routes"] = strings.Split(parts[1], ",")
						}
						endpoints = append(endpoints, m)
					}
					body["endpoints"] = endpoints
				}

				// Trigger paths
				if tp := req.GetString("trigger_paths", ""); tp != "" {
					paths := strings.Split(tp, ",")
					if gh, ok := body["github"].(map[string]any); ok {
						gh["triggerPaths"] = paths
					}
				}

				data, err := client.CreateApp(ctx, project, body)
				return jsonResult(data, err)
			})

			s.AddTool(mcp.NewTool("update_app",
				mcp.WithDescription(`Update application configuration. Only specified fields are changed.

Updatable fields: build_type, endpoints, github_branch, trigger_paths, build_options
Note: github_owner and github_repo cannot be changed after creation.`),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
				mcp.WithString("app", mcp.Required(), mcp.Description("App name")),
				mcp.WithString("build_type", mcp.Description("New build type: gradle|nodejs|react|vite|vue|nextjs|nextjs-export|go|rust|maven|django|flask|docker")),
				mcp.WithString("endpoints", mcp.Description(`JSON array of endpoint strings. Example: ["8080:api.dsmhs.kr","9090"]`)),
				mcp.WithString("github_branch", mcp.Description("New GitHub branch")),
				mcp.WithString("trigger_paths", mcp.Description("Comma-separated CI trigger paths, e.g. src/**,Dockerfile")),
				mcp.WithString("build_options", mcp.Description("JSON object of build-type specific options")),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				project, err := req.RequireString("project")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				appName, err := req.RequireString("app")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				// Fetch existing config as base
				existing, err := client.GetApp(ctx, project, appName)
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				body := existing
				body["name"] = appName

				if bt := req.GetString("build_type", ""); bt != "" {
					buildOpts := map[string]any{}
					if boStr := req.GetString("build_options", ""); boStr != "" {
						if e := json.Unmarshal([]byte(boStr), &buildOpts); e != nil {
							return mcp.NewToolResultError("invalid build_options JSON: " + e.Error()), nil
						}
					}
					body["build"] = map[string]any{bt: buildOpts}
				}

				if epStr := req.GetString("endpoints", ""); epStr != "" {
					var eps []string
					if e := json.Unmarshal([]byte(epStr), &eps); e != nil {
						return mcp.NewToolResultError("invalid endpoints JSON: " + e.Error()), nil
					}
					var endpoints []map[string]any
					for _, ep := range eps {
						parts := strings.SplitN(ep, ":", 2)
						port := 0
						if _, e := fmt.Sscanf(parts[0], "%d", &port); e != nil || port <= 0 {
							return mcp.NewToolResultError(fmt.Sprintf("invalid endpoint port in %q", ep)), nil
						}
						m := map[string]any{"port": port}
						if len(parts) == 2 && parts[1] != "" {
							m["routes"] = strings.Split(parts[1], ",")
						}
						endpoints = append(endpoints, m)
					}
					body["endpoints"] = endpoints
				}

				if branch := req.GetString("github_branch", ""); branch != "" {
					if gh, ok := body["github"].(map[string]any); ok {
						gh["branch"] = branch
					}
				}

				if tp := req.GetString("trigger_paths", ""); tp != "" {
					if gh, ok := body["github"].(map[string]any); ok {
						gh["triggerPaths"] = strings.Split(tp, ",")
					}
				}

				data, err := client.UpdateApp(ctx, project, appName, body)
				return jsonResult(data, err)
			})

		s.AddTool(mcp.NewTool("delete_app",
				mcp.WithDescription("Delete an application. Irreversible — also removes Vault secrets."),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
				mcp.WithString("app", mcp.Required(), mcp.Description("App name")),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				project, err := req.RequireString("project")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				app, err := req.RequireString("app")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				if err := client.DeleteApp(ctx, project, app); err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				return mcp.NewToolResultText(fmt.Sprintf("deleted app %q from project %q", app, project)), nil
			})

			// ── Env tools ─────────────────────────────────────────────────

			s.AddTool(mcp.NewTool("get_env",
				mcp.WithDescription("Get all environment variables for an app. Returns key-value map."),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
				mcp.WithString("app", mcp.Required(), mcp.Description("App name")),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				project, err := req.RequireString("project")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				app, err := req.RequireString("app")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				data, err := client.GetEnv(ctx, project, app)
				return jsonResult(data, err)
			})

			s.AddTool(mcp.NewTool("set_env",
				mcp.WithDescription("Set environment variables for an app (merges with existing, does NOT delete unspecified keys). Use delete_env to remove specific keys."),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
				mcp.WithString("app", mcp.Required(), mcp.Description("App name")),
				mcp.WithString("vars", mcp.Required(), mcp.Description(`JSON object of key-value pairs. Example: {"DB_HOST":"localhost","DB_PORT":"5432"}`)),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				project, err := req.RequireString("project")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				app, err := req.RequireString("app")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				varsStr, err := req.RequireString("vars")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				var vars map[string]string
				if e := json.Unmarshal([]byte(varsStr), &vars); e != nil {
					return mcp.NewToolResultError("invalid vars JSON: " + e.Error()), nil
				}
				data, err := client.PatchEnv(ctx, project, app, vars)
				return jsonResult(data, err)
			})

			s.AddTool(mcp.NewTool("delete_env",
				mcp.WithDescription("Delete specific environment variable keys from an app."),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
				mcp.WithString("app", mcp.Required(), mcp.Description("App name")),
				mcp.WithString("keys", mcp.Required(), mcp.Description(`JSON array of key names to delete. Example: ["OLD_KEY","UNUSED_VAR"]`)),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				project, err := req.RequireString("project")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				app, err := req.RequireString("app")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				keysStr, err := req.RequireString("keys")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				var keys []string
				if e := json.Unmarshal([]byte(keysStr), &keys); e != nil {
					return mcp.NewToolResultError("invalid keys JSON: " + e.Error()), nil
				}
				for _, k := range keys {
					if e := client.DeleteEnvKey(ctx, project, app, k); e != nil {
						return mcp.NewToolResultError(fmt.Sprintf("delete %s: %s", k, e.Error())), nil
					}
				}
				return mcp.NewToolResultText(fmt.Sprintf("deleted %d env key(s)", len(keys))), nil
			})

			// ── Addon tools ───────────────────────────────────────────────

			s.AddTool(mcp.NewTool("list_addons",
				mcp.WithDescription("List addons (databases/caches) for a project with provisioning status."),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				project, err := req.RequireString("project")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				data, err := client.ListAddons(ctx, project)
				return jsonResult(data, err)
			})

			s.AddTool(mcp.NewTool("create_addon",
				mcp.WithDescription(`Provision a database or cache addon. Takes 1-2 minutes to become ready.

CONSTRAINTS:
- type: mysql | postgresql | redis | mongodb | kafka | rabbitmq | opensearch | elasticsearch | qdrant
- storage: must be less than 4Gi. Default: 2Gi. Examples: 500Mi, 1Gi, 2Gi, 3Gi`),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
				mcp.WithString("name", mcp.Required(), mcp.Description("Addon name")),
				mcp.WithString("addon_type", mcp.Required(), mcp.Description("mysql|postgresql|redis|mongodb|kafka|rabbitmq|opensearch|elasticsearch|qdrant")),
				mcp.WithString("storage", mcp.Description("Storage size, must be < 4Gi (default: 2Gi). Examples: 500Mi, 1Gi, 2Gi")),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				project, err := req.RequireString("project")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				name, err := req.RequireString("name")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				addonType, err := req.RequireString("addon_type")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				storage := req.GetString("storage", "2Gi")
				body := map[string]string{
					"name":    name,
					"type":    addonType,
					"storage": storage,
				}
				data, err := client.CreateAddon(ctx, project, body)
				return jsonResult(data, err)
			})

			s.AddTool(mcp.NewTool("delete_addon",
				mcp.WithDescription("Delete an addon and its persistent storage. Irreversible."),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
				mcp.WithString("addon", mcp.Required(), mcp.Description("Addon name")),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				project, err := req.RequireString("project")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				addon, err := req.RequireString("addon")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				if err := client.DeleteAddon(ctx, project, addon); err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				return mcp.NewToolResultText(fmt.Sprintf("deleted addon %q", addon)), nil
			})

			s.AddTool(mcp.NewTool("get_addon_connection",
				mcp.WithDescription("Get connection info for an addon (host, port, password). The password is the wstunnel access key for tunneling."),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
				mcp.WithString("addon", mcp.Required(), mcp.Description("Addon name")),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				project, err := req.RequireString("project")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				addon, err := req.RequireString("addon")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				data, err := client.GetAddonConnection(ctx, project, addon)
				return jsonResult(data, err)
			})

			// ── Deploy & logs ─────────────────────────────────────────────

			s.AddTool(mcp.NewTool("deploy",
				mcp.WithDescription("Trigger re-deploy with the latest commit. CI pipeline must be ready (ciReady=true in get_app_status). Returns build workflow name."),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
				mcp.WithString("app", mcp.Required(), mcp.Description("App name")),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				project, err := req.RequireString("project")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				app, err := req.RequireString("app")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				data, err := client.RedeployApp(ctx, project, app)
				return jsonResult(data, err)
			})

			s.AddTool(mcp.NewTool("get_logs",
				mcp.WithDescription("Get recent runtime logs for an app. Returns last N lines as text. For real-time streaming, use 'xquare logs <app> -f' CLI command instead."),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
				mcp.WithString("app", mcp.Required(), mcp.Description("App name")),
				mcp.WithNumber("tail", mcp.Description("Number of lines from end (default: 100, max: 500)")),
				mcp.WithString("since", mcp.Description("Show logs since duration, e.g. 1h, 30m")),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				project, err := req.RequireString("project")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				appName, err := req.RequireString("app")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				tail := int64(req.GetFloat("tail", 100))
				if tail > 500 {
					tail = 500
				}
				since := req.GetString("since", "")
				resp, err := client.StreamLogs(ctx, project, appName, tail, false, since)
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				defer resp.Body.Close()
				if resp.StatusCode >= 400 {
					b, _ := io.ReadAll(resp.Body)
					return mcp.NewToolResultError(fmt.Sprintf("server error %d: %s", resp.StatusCode, string(b))), nil
				}
				var lines []string
				scanner := bufio.NewScanner(resp.Body)
				for scanner.Scan() {
					lines = append(lines, scanner.Text())
				}
				return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
			})

			s.AddTool(mcp.NewTool("schema",
				mcp.WithDescription("Return the full xquare CLI command schema with all constraints, valid values, and examples. Call this first to understand all available commands before doing anything else."),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				s := schema.BuildSchema()
				b, err := json.MarshalIndent(s, "", "  ")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				return mcp.NewToolResultText(string(b)), nil
			})

			fmt.Fprintln(os.Stderr, "xquare MCP server started (stdio)")
			return server.ServeStdio(s)
		},
	}

	cmd.Flags().BoolVar(&installClaude, "claude", false, "register xquare MCP server in Claude Desktop config")
	cmd.Flags().BoolVar(&installCursor, "cursor", false, "register xquare MCP server in Cursor config")
	cmd.Flags().BoolVar(&installVSCode, "vscode", false, "register xquare MCP server in VS Code config")
	return cmd
}

// registerMCP installs xquare as an MCP server in the specified IDE configs.
func registerMCP(claude, cursor, vscode bool) error {
	// Resolve xquare binary path
	xquarePath, err := exec.LookPath("xquare")
	if err != nil {
		// Fall back to current executable
		xquarePath, _ = os.Executable()
	}

	mcpEntry := map[string]any{
		"command": xquarePath,
		"args":    []string{"mcp"},
	}

	var registered []string

	if claude {
		if err := writeMCPConfig(claudeConfigPath(), mcpEntry); err != nil {
			return fmt.Errorf("claude: %w", err)
		}
		registered = append(registered, "Claude Desktop")
	}
	if cursor {
		if err := writeMCPConfig(cursorConfigPath(), mcpEntry); err != nil {
			return fmt.Errorf("cursor: %w", err)
		}
		registered = append(registered, "Cursor")
	}
	if vscode {
		if err := writeMCPConfig(vscodeConfigPath(), mcpEntry); err != nil {
			return fmt.Errorf("vscode: %w", err)
		}
		registered = append(registered, "VS Code")
	}

	for _, ide := range registered {
		output.Success(fmt.Sprintf("registered xquare MCP server in %s", ide))
	}
	output.Info("restart your IDE to activate")
	return nil
}

// writeMCPConfig merges the xquare MCP entry into the IDE's config JSON file.
func writeMCPConfig(configPath string, entry map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}

	var cfg map[string]any
	if data, err := os.ReadFile(configPath); err == nil {
		_ = json.Unmarshal(data, &cfg)
	}
	if cfg == nil {
		cfg = map[string]any{}
	}

	servers, _ := cfg["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	servers["xquare"] = entry
	cfg["mcpServers"] = servers

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}

func claudeConfigPath() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "Claude", "claude_desktop_config.json")
	default: // linux
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			return filepath.Join(xdg, "Claude", "claude_desktop_config.json")
		}
		return filepath.Join(home, ".config", "Claude", "claude_desktop_config.json")
	}
}

func cursorConfigPath() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Cursor", "User", "globalStorage", "cursor.mcp", "settings.json")
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "Cursor", "User", "globalStorage", "cursor.mcp", "settings.json")
	default:
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			return filepath.Join(xdg, "Cursor", "User", "globalStorage", "cursor.mcp", "settings.json")
		}
		return filepath.Join(home, ".config", "Cursor", "User", "globalStorage", "cursor.mcp", "settings.json")
	}
}

func vscodeConfigPath() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Code", "User", "mcp.json")
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "Code", "User", "mcp.json")
	default:
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			return filepath.Join(xdg, "Code", "User", "mcp.json")
		}
		return filepath.Join(home, ".config", "Code", "User", "mcp.json")
	}
}

func jsonResult(data any, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(b)), nil
}
