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
	"gopkg.in/yaml.v3"

	"github.com/team-xquare/xquare-cli/cmd/schema"
	"github.com/team-xquare/xquare-cli/internal/api"
	"github.com/team-xquare/xquare-cli/internal/config"
	"github.com/team-xquare/xquare-cli/internal/output"
)

func NewMCPCmd() *cobra.Command {
	var (
		installClaude, installCursor, installVSCode             bool
		installClaudeCode, installWindsurf, installZed          bool
		installContinue, installCline, installRoo, installGoose bool
	)

	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Start MCP server (for AI agent integration)",
		Long: `Start xquare as an MCP (Model Context Protocol) server.

Use a registration flag to add xquare as an MCP server in your AI tool:
  --claude        Claude Desktop
  --claude-code   Claude Code CLI
  --cursor        Cursor
  --vscode        VS Code (GitHub Copilot)
  --windsurf      Windsurf
  --zed           Zed
  --continue      Continue.dev
  --cline         Cline (VS Code extension)
  --roo           Roo Code (VS Code extension)
  --goose         Goose (Block)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Handle IDE registration flags
			if installClaude || installCursor || installVSCode ||
				installClaudeCode || installWindsurf || installZed ||
				installContinue || installCline || installRoo || installGoose {
				return registerMCP(registerOpts{
					claude:      installClaude,
					cursor:      installCursor,
					vscode:      installVSCode,
					claudeCode:  installClaudeCode,
					windsurf:    installWindsurf,
					zed:         installZed,
					continueApp: installContinue,
					cline:       installCline,
					roo:         installRoo,
					goose:       installGoose,
				})
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
				mcp.WithDescription(`⚠️  DESTRUCTIVE — IRREVERSIBLE. Deletes a project AND ALL its apps, addons, environment variables, and persistent storage forever.

BEFORE calling this tool you MUST:
1. Tell the user exactly what will be deleted
2. Ask the user to explicitly confirm with "yes"
3. Only proceed after receiving clear confirmation
4. Set confirm="yes" to execute`),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name to delete")),
				mcp.WithString("confirm", mcp.Required(), mcp.Description(`Must be exactly "yes" — only set this after the user has explicitly confirmed deletion`)),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				if req.GetString("confirm", "") != "yes" {
					return mcp.NewToolResultError("deletion cancelled: confirm must be \"yes\". Ask the user to explicitly confirm before proceeding."), nil
				}
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

DEPLOYMENT FLOW after create_app:
1. Wait ~2-3 minutes for CI infrastructure to initialize (ciReady=true in get_app_status)
2. If code is already on GitHub (was pushed before this app was created): call trigger ONCE — the webhook missed the commit
3. If code will be pushed after this step: just git push — CI runs automatically, do NOT call trigger
4. For ALL subsequent deployments: git push → CI runs automatically — ⛔ NEVER call trigger after a git push`),
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
				mcp.WithDescription(`⚠️  DESTRUCTIVE — IRREVERSIBLE. Deletes the app, its CI/CD pipeline, all environment variables (Vault secrets), and removes it from the GitOps repo forever.

BEFORE calling this tool you MUST:
1. Tell the user exactly what will be deleted
2. Ask the user to explicitly confirm with "yes"
3. Only proceed after receiving clear confirmation
4. Set confirm="yes" to execute`),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
				mcp.WithString("app", mcp.Required(), mcp.Description("App name")),
				mcp.WithString("confirm", mcp.Required(), mcp.Description(`Must be exactly "yes" — only set this after the user has explicitly confirmed deletion`)),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				if req.GetString("confirm", "") != "yes" {
					return mcp.NewToolResultError("deletion cancelled: confirm must be \"yes\". Ask the user to explicitly confirm before proceeding."), nil
				}
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
				mcp.WithDescription("Delete specific environment variable keys from an app. ⚠️  If the app is running, missing env vars may cause crashes on next deploy. Confirm the keys with the user before deleting."),
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
				mcp.WithDescription(`⚠️  DESTRUCTIVE — IRREVERSIBLE. Deletes the addon AND ALL its persistent data (database contents, files) forever. Data cannot be recovered.

BEFORE calling this tool you MUST:
1. Warn the user that ALL DATA in the addon will be permanently lost
2. Ask the user to explicitly confirm with "yes"
3. Only proceed after receiving clear confirmation
4. Set confirm="yes" to execute`),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
				mcp.WithString("addon", mcp.Required(), mcp.Description("Addon name")),
				mcp.WithString("confirm", mcp.Required(), mcp.Description(`Must be exactly "yes" — only set this after the user has explicitly confirmed deletion`)),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				if req.GetString("confirm", "") != "yes" {
					return mcp.NewToolResultError("deletion cancelled: confirm must be \"yes\". Ask the user to explicitly confirm before proceeding."), nil
				}
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

			s.AddTool(mcp.NewTool("get_addon_status",
				mcp.WithDescription(`Get status and connection info for an addon. Returns ready-to-use env var values.

⚠️  CRITICAL — xquare addons have NO PASSWORD. Never ask the user for a DB password. Never look for one.
The response includes all env vars you need. Just use them as-is.

Response fields are ready-to-use:
  db_host, db_port, db_user, db_password (always empty), db_name

For local tunneling use 'xquare addon tunnel' CLI command.`),
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
				if err != nil {
					return jsonResult(nil, err)
				}
				// Strip internal tunnel credentials from MCP response
				addonType := fmt.Sprintf("%v", data["type"])
				defaultUser := map[string]string{
					"postgresql": "postgres",
					"mysql":      "root",
					"mongodb":    "(no auth)",
					"redis":      "(no auth)",
				}[addonType]
				if defaultUser == "" {
					defaultUser = "(no auth)"
				}
				safe := map[string]any{
					"name":        addon,
					"type":        addonType,
					"ready":       data["ready"],
					"port":        data["port"],
					"db_host":     addon,
					"db_port":     data["port"],
					"db_user":     defaultUser,
					"db_password": "(none — no password required)",
					"db_name":     addon,
					"note":        "No password needed. Connect directly using the addon name as hostname inside your app.",
				}
				return jsonResult(safe, nil)
			})

			// ── Deploy & logs ─────────────────────────────────────────────

			s.AddTool(mcp.NewTool("trigger",
				mcp.WithDescription(`Force re-run CI/CD with the latest commit.

⚠️  WHEN TO USE trigger (ONLY these cases):
1. App was just created AND code was already on GitHub before app creation (initial build only)
2. The automatic webhook failed (you pushed but nothing happened after 5+ minutes)
3. You need to re-deploy without making a code change

⛔  DO NOT use trigger:
- After a git push (CI runs automatically — webhook handles it)
- Just because you want to "start a build" after pushing
- Repeatedly to check if build works (push once, wait, check logs)

CI pipeline must be ready (ciReady=true in get_app_status) before calling trigger.`),
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
				data, err := client.TriggerApp(ctx, project, app)
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

			// ── Members ───────────────────────────────────────────────────

			s.AddTool(mcp.NewTool("list_members",
				mcp.WithDescription("List members (owners) of a project."),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				project, err := req.RequireString("project")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				data, err := client.ListMembers(ctx, project)
				return jsonResult(data, err)
			})

			s.AddTool(mcp.NewTool("add_member",
				mcp.WithDescription("Add a GitHub user as a project member by their GitHub username."),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
				mcp.WithString("username", mcp.Required(), mcp.Description("GitHub username to add")),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				project, err := req.RequireString("project")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				username, err := req.RequireString("username")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				if err := client.AddMember(ctx, project, username); err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				return mcp.NewToolResultText(fmt.Sprintf("added %q to project %q", username, project)), nil
			})

			s.AddTool(mcp.NewTool("remove_member",
				mcp.WithDescription(`⚠️  Removes a member's access to the project. They will lose ability to deploy, manage apps, and view resources.

BEFORE calling this tool you MUST:
1. Confirm the username with the user
2. Ask the user to explicitly confirm with "yes"
3. Set confirm="yes" to execute`),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
				mcp.WithString("username", mcp.Required(), mcp.Description("GitHub username to remove")),
				mcp.WithString("confirm", mcp.Required(), mcp.Description(`Must be exactly "yes" — only set this after the user has explicitly confirmed`)),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				if req.GetString("confirm", "") != "yes" {
					return mcp.NewToolResultError("cancelled: confirm must be \"yes\". Ask the user to explicitly confirm before proceeding."), nil
				}
				project, err := req.RequireString("project")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				username, err := req.RequireString("username")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				if err := client.RemoveMember(ctx, project, username); err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				return mcp.NewToolResultText(fmt.Sprintf("removed %q from project %q", username, project)), nil
			})

			// ── Build tools ───────────────────────────────────────────────

			s.AddTool(mcp.NewTool("list_builds",
				mcp.WithDescription("List recent CI/CD build history for an app. Returns build ID, status (running/success/failed), and timestamps."),
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
				data, err := client.ListBuilds(ctx, project, app)
				return jsonResult(data, err)
			})

			s.AddTool(mcp.NewTool("get_build_logs",
				mcp.WithDescription(`Get CI/CD build logs for a specific build.

⚠️  If build was just triggered (< 30s ago), the pod may still be initializing.
If you get status "initializing", wait 20-30 seconds and call again — do NOT retry immediately in a loop.
Use list_builds to get build IDs, or omit build_id to get the latest build logs.`),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
				mcp.WithString("app", mcp.Required(), mcp.Description("App name")),
				mcp.WithString("build_id", mcp.Description("Build workflow ID (e.g. my-app-ci-abc12). Omit for latest build.")),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				project, err := req.RequireString("project")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				appName, err := req.RequireString("app")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				buildID := req.GetString("build_id", "")
				if buildID == "" {
					builds, err := client.ListBuilds(ctx, project, appName)
					if err != nil {
						return mcp.NewToolResultError(err.Error()), nil
					}
					if len(builds) == 0 {
						return mcp.NewToolResultError("no builds found"), nil
					}
					buildID = fmt.Sprintf("%v", builds[0]["id"])
				}
				resp, err := client.StreamBuildLogs(ctx, project, appName, buildID, false)
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

			// ── Meta ──────────────────────────────────────────────────────

			s.AddTool(mcp.NewTool("whoami",
				mcp.WithDescription("Get the currently authenticated user information (username, server URL)."),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				cfg, err := config.LoadGlobal()
				if err != nil || cfg.Token == "" {
					return mcp.NewToolResultError("not logged in"), nil
				}
				return jsonResult(map[string]string{
					"username": cfg.Username,
					"server":   cfg.ServerURL,
				}, nil)
			})

			s.AddTool(mcp.NewTool("get_dashboard_url",
				mcp.WithDescription("Get the Grafana monitoring dashboard URL and login credentials for a project, app, or addon. Use type='project' to get the project-level Grafana login info (URL + username + password). Use type='app' or type='addon' with a name for a specific dashboard URL."),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
				mcp.WithString("name", mcp.Description("App or addon name (not required when type is 'project')")),
				mcp.WithString("type", mcp.Description(`"project" (default) for project Grafana credentials, "app" for app dashboard URL, "addon" for addon dashboard URL`)),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				project := req.GetString("project", "")
				name := req.GetString("name", "")
				kind := req.GetString("type", "project")
				if project == "" {
					return mcp.NewToolResultError("project is required"), nil
				}
				if kind == "project" {
					info, err := client.GetDashboard(ctx, project)
					if err != nil {
						return mcp.NewToolResultError(err.Error()), nil
					}
					return jsonResult(info, nil)
				}
				if name == "" {
					return mcp.NewToolResultError("name is required for type app or addon"), nil
				}
				var dashURL string
				if kind == "addon" {
					dashURL = fmt.Sprintf("https://%s-observability-dashboard.dsmhs.kr/d/addon-%s", project, name)
				} else {
					dashURL = fmt.Sprintf("https://%s-observability-dashboard.dsmhs.kr/d/app-%s", project, name)
				}
				return jsonResult(map[string]string{
					"url":     dashURL,
					"project": project,
					"name":    name,
					"type":    kind,
				}, nil)
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

	cmd.Flags().BoolVar(&installClaude, "claude", false, "register in Claude Desktop config")
	cmd.Flags().BoolVar(&installCursor, "cursor", false, "register in Cursor config")
	cmd.Flags().BoolVar(&installVSCode, "vscode", false, "register in VS Code (Copilot) config")
	cmd.Flags().BoolVar(&installClaudeCode, "claude-code", false, "register in Claude Code CLI config (~/.claude.json)")
	cmd.Flags().BoolVar(&installWindsurf, "windsurf", false, "register in Windsurf config")
	cmd.Flags().BoolVar(&installZed, "zed", false, "register in Zed editor config")
	cmd.Flags().BoolVar(&installContinue, "continue", false, "register in Continue.dev config")
	cmd.Flags().BoolVar(&installCline, "cline", false, "register in Cline (VS Code extension) config")
	cmd.Flags().BoolVar(&installRoo, "roo", false, "register in Roo Code (VS Code extension) config")
	cmd.Flags().BoolVar(&installGoose, "goose", false, "register in Goose (Block) config")

	return cmd
}

type registerOpts struct {
	claude      bool
	cursor      bool
	vscode      bool
	claudeCode  bool
	windsurf    bool
	zed         bool
	continueApp bool
	cline       bool
	roo         bool
	goose       bool
}

// registerMCP installs xquare as an MCP server in the specified IDE/tool configs.
func registerMCP(opts registerOpts) error {
	xquarePath, err := exec.LookPath("xquare")
	if err != nil {
		xquarePath, _ = os.Executable()
	}

	// Standard entry (mcpServers format)
	stdEntry := map[string]any{
		"command": xquarePath,
		"args":    []string{"mcp"},
	}
	// Claude Code entry adds type field
	claudeCodeEntry := map[string]any{
		"type":    "stdio",
		"command": xquarePath,
		"args":    []string{"mcp"},
	}
	// Cline/Roo entry adds disabled + alwaysAllow
	clineEntry := map[string]any{
		"command":     xquarePath,
		"args":        []string{"mcp"},
		"disabled":    false,
		"alwaysAllow": []string{},
	}

	type registration struct {
		name string
		fn   func() error
	}

	var regs []registration

	if opts.claude {
		regs = append(regs, registration{"Claude Desktop", func() error {
			return writeMCPServersConfig(claudeDesktopConfigPath(), stdEntry)
		}})
	}
	if opts.claudeCode {
		regs = append(regs, registration{"Claude Code CLI", func() error {
			return writeMCPServersConfig(claudeCodeConfigPath(), claudeCodeEntry)
		}})
	}
	if opts.cursor {
		regs = append(regs, registration{"Cursor", func() error {
			return writeMCPServersConfig(cursorConfigPath(), stdEntry)
		}})
	}
	if opts.vscode {
		regs = append(regs, registration{"VS Code", func() error {
			return writeVSCodeConfig(vscodeConfigPath(), stdEntry)
		}})
	}
	if opts.windsurf {
		regs = append(regs, registration{"Windsurf", func() error {
			return writeMCPServersConfig(windsurfConfigPath(), stdEntry)
		}})
	}
	if opts.zed {
		regs = append(regs, registration{"Zed", func() error {
			return writeZedConfig(zedConfigPath(), xquarePath)
		}})
	}
	if opts.continueApp {
		regs = append(regs, registration{"Continue.dev", func() error {
			return writeContinueConfig(continueConfigPath(), xquarePath)
		}})
	}
	if opts.cline {
		regs = append(regs, registration{"Cline", func() error {
			return writeMCPServersConfig(clineConfigPath(), clineEntry)
		}})
	}
	if opts.roo {
		regs = append(regs, registration{"Roo Code", func() error {
			return writeMCPServersConfig(rooConfigPath(), clineEntry)
		}})
	}
	if opts.goose {
		regs = append(regs, registration{"Goose", func() error {
			return writeGooseConfig(gooseConfigPath(), xquarePath)
		}})
	}

	for _, r := range regs {
		if err := r.fn(); err != nil {
			return fmt.Errorf("%s: %w", r.name, err)
		}
		output.Success(fmt.Sprintf("registered xquare MCP server in %s", r.name))
	}
	if len(regs) > 0 {
		output.Info("restart your IDE/tool to activate")
	}
	return nil
}

// writeMCPServersConfig merges entry under "mcpServers" key in a JSON config file.
func writeMCPServersConfig(configPath string, entry map[string]any) error {
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

// writeVSCodeConfig merges entry under "servers" key (VS Code uses "servers" not "mcpServers").
func writeVSCodeConfig(configPath string, entry map[string]any) error {
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
	servers, _ := cfg["servers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	servers["xquare"] = entry
	cfg["servers"] = servers
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}

// writeZedConfig merges into Zed's settings.json under "context_servers" with its unique nested structure.
func writeZedConfig(configPath string, xquarePath string) error {
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
	servers, _ := cfg["context_servers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	servers["xquare"] = map[string]any{
		"command": map[string]any{
			"path": xquarePath,
			"args": []string{"mcp"},
			"env":  map[string]any{},
		},
		"settings": map[string]any{},
	}
	cfg["context_servers"] = servers
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}

// writeContinueConfig adds xquare to Continue.dev's config.yaml under mcpServers array.
func writeContinueConfig(configPath string, xquarePath string) error {
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}

	type mcpServer struct {
		Name    string   `yaml:"name"`
		Command string   `yaml:"command"`
		Args    []string `yaml:"args"`
	}
	type continueConfig struct {
		MCPServers []mcpServer            `yaml:"mcpServers,omitempty"`
		Rest       map[string]interface{} `yaml:",inline"`
	}

	var cfg continueConfig
	if data, err := os.ReadFile(configPath); err == nil {
		_ = yaml.Unmarshal(data, &cfg)
	}

	// Remove existing xquare entry if present
	filtered := cfg.MCPServers[:0]
	for _, s := range cfg.MCPServers {
		if s.Name != "xquare" {
			filtered = append(filtered, s)
		}
	}
	cfg.MCPServers = append(filtered, mcpServer{
		Name:    "xquare",
		Command: xquarePath,
		Args:    []string{"mcp"},
	})

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}

// writeGooseConfig adds xquare to Goose's config.yaml under extensions.
func writeGooseConfig(configPath string, xquarePath string) error {
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}

	var cfg map[string]any
	if data, err := os.ReadFile(configPath); err == nil {
		_ = yaml.Unmarshal(data, &cfg)
	}
	if cfg == nil {
		cfg = map[string]any{}
	}

	exts, _ := cfg["extensions"].(map[string]any)
	if exts == nil {
		exts = map[string]any{}
	}
	exts["xquare"] = map[string]any{
		"name":    "xquare",
		"cmd":     xquarePath,
		"args":    []string{"mcp"},
		"enabled": true,
		"type":    "stdio",
		"timeout": 300,
		"envs":    map[string]any{},
	}
	cfg["extensions"] = exts

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}

func claudeDesktopConfigPath() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "Claude", "claude_desktop_config.json")
	default:
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			return filepath.Join(xdg, "Claude", "claude_desktop_config.json")
		}
		return filepath.Join(home, ".config", "Claude", "claude_desktop_config.json")
	}
}

func claudeCodeConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude.json")
}

func cursorConfigPath() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin", "windows":
		return filepath.Join(home, ".cursor", "mcp.json")
	default:
		return filepath.Join(home, ".cursor", "mcp.json")
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

func windsurfConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codeium", "windsurf", "mcp_config.json")
}

func zedConfigPath() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Zed", "settings.json")
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "Zed", "settings.json")
	default:
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			return filepath.Join(xdg, "zed", "settings.json")
		}
		return filepath.Join(home, ".config", "zed", "settings.json")
	}
}

func continueConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".continue", "config.yaml")
}

func clineConfigPath() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Code", "User", "globalStorage", "saoudrizwan.claude-dev", "settings", "cline_mcp_settings.json")
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "Code", "User", "globalStorage", "saoudrizwan.claude-dev", "settings", "cline_mcp_settings.json")
	default:
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			return filepath.Join(xdg, "Code", "User", "globalStorage", "saoudrizwan.claude-dev", "settings", "cline_mcp_settings.json")
		}
		return filepath.Join(home, ".config", "Code", "User", "globalStorage", "saoudrizwan.claude-dev", "settings", "cline_mcp_settings.json")
	}
}

func rooConfigPath() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Code", "User", "globalStorage", "rooveterinaryinc.roo-cline", "settings", "cline_mcp_settings.json")
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "Code", "User", "globalStorage", "rooveterinaryinc.roo-cline", "settings", "cline_mcp_settings.json")
	default:
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			return filepath.Join(xdg, "Code", "User", "globalStorage", "rooveterinaryinc.roo-cline", "settings", "cline_mcp_settings.json")
		}
		return filepath.Join(home, ".config", "Code", "User", "globalStorage", "rooveterinaryinc.roo-cline", "settings", "cline_mcp_settings.json")
	}
}

func gooseConfigPath() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "Block", "goose", "config", "config.yaml")
	default:
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			return filepath.Join(xdg, "goose", "config.yaml")
		}
		return filepath.Join(home, ".config", "goose", "config.yaml")
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
