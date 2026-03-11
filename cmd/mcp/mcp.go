package mcp

import (
	"context"
	"fmt"
	"os"

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
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			c := api.New(cfg.ServerURL, cfg.Token)
			return runMCPServer(c)
		},
	}
}

func runMCPServer(c *api.Client) error {
	s := server.NewMCPServer("xquare", "1.0.0",
		server.WithToolCapabilities(true),
	)

	// list_projects
	s.AddTool(mcp.NewTool("list_projects",
		mcp.WithDescription("List all xquare projects"),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projects, err := c.ListProjects(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("%v", projects)), nil
	})

	// get_project
	s.AddTool(mcp.NewTool("get_project",
		mcp.WithDescription("Get project details"),
		mcp.WithString("project", mcp.Required(), mcp.Description("project name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		project, err := req.RequireString("project")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		p, err := c.GetProject(ctx, project)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("%v", p)), nil
	})

	// get_app_status
	s.AddTool(mcp.NewTool("get_app_status",
		mcp.WithDescription("Get deployment status of an application"),
		mcp.WithString("project", mcp.Required(), mcp.Description("project name")),
		mcp.WithString("app", mcp.Required(), mcp.Description("application name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		project, err := req.RequireString("project")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		app, err := req.RequireString("app")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		status, err := c.GetAppStatus(ctx, project, app)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("%v", status)), nil
	})

	// get_env
	s.AddTool(mcp.NewTool("get_env",
		mcp.WithDescription("Get environment variables for an application"),
		mcp.WithString("project", mcp.Required(), mcp.Description("project name")),
		mcp.WithString("app", mcp.Required(), mcp.Description("application name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		project, err := req.RequireString("project")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		app, err := req.RequireString("app")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		envs, err := c.GetEnv(ctx, project, app)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("%v", envs)), nil
	})

	// list_addons
	s.AddTool(mcp.NewTool("list_addons",
		mcp.WithDescription("List addons in a project"),
		mcp.WithString("project", mcp.Required(), mcp.Description("project name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		project, err := req.RequireString("project")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		addons, err := c.ListAddons(ctx, project)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("%v", addons)), nil
	})

	stdio := server.NewStdioServer(s)
	return stdio.Listen(context.Background(), os.Stdin, os.Stdout)
}
