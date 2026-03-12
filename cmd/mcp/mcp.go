package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/team-xquare/xquare-cli/internal/api"
	"github.com/team-xquare/xquare-cli/internal/config"
)

func NewMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start MCP server (for AI agent integration)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadGlobal()
			if err != nil || cfg.Token == "" {
				return fmt.Errorf("not logged in — run: xquare login")
			}
			client := api.New(cfg.ServerURL, cfg.Token)

			s := server.NewMCPServer("xquare", "1.0.0",
				server.WithToolCapabilities(true),
			)

			s.AddTool(mcp.NewTool("list_projects",
				mcp.WithDescription("List all projects"),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				data, err := client.ListProjects(ctx)
				return jsonResult(data, err)
			})

			s.AddTool(mcp.NewTool("get_project",
				mcp.WithDescription("Get project details including all apps and addons"),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				project, err := req.RequireString("project")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				data, err := client.GetProject(ctx, project)
				return jsonResult(data, err)
			})

			s.AddTool(mcp.NewTool("list_apps",
				mcp.WithDescription("List applications in a project"),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				project, err := req.RequireString("project")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				data, err := client.ListApps(ctx, project)
				return jsonResult(data, err)
			})

			s.AddTool(mcp.NewTool("get_app_status",
				mcp.WithDescription("Get deployment status (replicas, pods, hash)"),
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

			s.AddTool(mcp.NewTool("get_env",
				mcp.WithDescription("Get environment variables for an app"),
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
				mcp.WithDescription("Set environment variables (merges by default). Use dry_run=true to preview."),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
				mcp.WithString("app", mcp.Required(), mcp.Description("App name")),
				mcp.WithString("vars", mcp.Required(), mcp.Description(`JSON object, e.g. {"KEY":"value"}`)),
				mcp.WithBoolean("dry_run", mcp.Description("Preview only, default true")),
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
				dryRun := req.GetBool("dry_run", true)
				var vars map[string]string
				if e := json.Unmarshal([]byte(varsStr), &vars); e != nil {
					return mcp.NewToolResultError("invalid vars JSON: " + e.Error()), nil
				}
				if dryRun {
					keys := make([]string, 0, len(vars))
					for k := range vars {
						keys = append(keys, k)
					}
					msg := fmt.Sprintf("[dry_run] would set %d var(s) on %s/%s: %s", len(vars), project, app, strings.Join(keys, ", "))
					return mcp.NewToolResultText(msg), nil
				}
				data, err := client.PatchEnv(ctx, project, app, vars)
				return jsonResult(data, err)
			})

			s.AddTool(mcp.NewTool("list_addons",
				mcp.WithDescription("List addons for a project"),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				project, err := req.RequireString("project")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				data, err := client.ListAddons(ctx, project)
				return jsonResult(data, err)
			})

			s.AddTool(mcp.NewTool("get_addon_connection",
				mcp.WithDescription("Get connection info for a database/cache addon"),
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

			s.AddTool(mcp.NewTool("deploy",
				mcp.WithDescription("Trigger re-deploy with latest commit"),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
				mcp.WithString("app", mcp.Required(), mcp.Description("App name")),
				mcp.WithString("github_token", mcp.Required(), mcp.Description("GitHub personal access token")),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				project, err := req.RequireString("project")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				app, err := req.RequireString("app")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				token, err := req.RequireString("github_token")
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				data, err := client.RedeployApp(ctx, project, app, token)
				return jsonResult(data, err)
			})

			fmt.Fprintln(os.Stderr, "xquare MCP server started (stdio)")
			return server.ServeStdio(s)
		},
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
