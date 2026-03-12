package app

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/team-xquare/xquare-cli/internal/api"
	"github.com/team-xquare/xquare-cli/internal/output"
)

func NewAppCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "app",
		Short:   "Manage applications",
		Aliases: []string{"a"},
	}
	cmd.AddCommand(
		newListCmd(),
		newGetCmd(),
		newStatusCmd(),
		newCreateCmd(),
		newDeleteCmd(),
	)
	return cmd
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List applications in a project",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			c := api.FromCmd(cmd)
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}

			apps, err := c.ListApps(cmd.Context(), project)
			if err != nil {
				return err
			}
			if api.IsJSON(cmd) {
				return output.JSON(apps)
			}
			if len(apps) == 0 {
				output.Info("no apps found")
				return nil
			}
			rows := make([][]string, 0, len(apps))
			for _, a := range apps {
				name := fmt.Sprintf("%v", a["name"])
				rows = append(rows, []string{name})
			}
			output.Table([]string{"NAME"}, rows)
			return nil
		},
	}
}

func newGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <app>",
		Short: "Show application details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := api.FromCmd(cmd)
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			a, err := c.GetApp(cmd.Context(), project, args[0])
			if err != nil {
				return err
			}
			return output.JSON(a)
		},
	}
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <app>",
		Short: "Show deployment status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := api.FromCmd(cmd)
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			status, err := c.GetAppStatus(cmd.Context(), project, args[0])
			if err != nil {
				return err
			}
			if api.IsJSON(cmd) {
				return output.JSON(status)
			}
			// Human-friendly output
			appStatus := fmt.Sprintf("%v", status["status"])
			hash := fmt.Sprintf("%v", status["hash"])
			if len(hash) > 8 {
				hash = hash[:8]
			}
			output.Info(fmt.Sprintf("Status: %s", appStatus))
			output.Info(fmt.Sprintf("Hash:   %s", hash))
			if pods, ok := status["pods"]; ok {
				output.Info(fmt.Sprintf("Pods:   %v", pods))
			}
			return nil
		},
	}
}

func newCreateCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "create <app>",
		Short: "Create a new application",
		Long: `Create a new application in a project.
Build type must be specified with --build-type flag.
Example: xquare app create myapp --build-type gradle --repo myrepo --branch main --owner team-myteam --installation-id 62433388`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			appName := args[0]

			buildType, _ := cmd.Flags().GetString("build-type")
			owner, _ := cmd.Flags().GetString("owner")
			repo, _ := cmd.Flags().GetString("repo")
			branch, _ := cmd.Flags().GetString("branch")
			installID, _ := cmd.Flags().GetString("installation-id")
			port, _ := cmd.Flags().GetInt("port")
			routes, _ := cmd.Flags().GetStringSlice("routes")

			if dryRun {
				output.Info(fmt.Sprintf("[dry-run] would create app %s in project %s", appName, project))
				return nil
			}

			body := buildAppBody(appName, buildType, owner, repo, branch, installID, port, routes, cmd)
			c := api.FromCmd(cmd)
			result, err := c.CreateApp(cmd.Context(), project, body)
			if err != nil {
				return err
			}
			if api.IsJSON(cmd) {
				return output.JSON(result)
			}
			output.Success(fmt.Sprintf("created app %s in project %s", appName, project))
			return nil
		},
	}
	cmd.Flags().String("build-type", "", "build type: gradle|nodejs|react|vite|vue|nextjs|nextjs-export|go|rust|maven|django|flask|docker")
	cmd.Flags().String("owner", "team-xquare", "GitHub owner")
	cmd.Flags().String("repo", "", "GitHub repository name")
	cmd.Flags().String("branch", "main", "GitHub branch")
	cmd.Flags().String("installation-id", "62433388", "GitHub App installation ID")
	cmd.Flags().Int("port", 8080, "application port")
	cmd.Flags().StringSlice("routes", []string{}, "HTTP routes (domain names)")
	// build-type specific flags
	cmd.Flags().String("java-version", "17", "Java version (gradle/maven)")
	cmd.Flags().String("node-version", "20", "Node.js version")
	cmd.Flags().String("go-version", "1.21", "Go version")
	cmd.Flags().String("python-version", "3.11", "Python version")
	cmd.Flags().String("build-command", "", "build command override")
	cmd.Flags().String("start-command", "", "start command (nodejs/nextjs/django/flask)")
	cmd.Flags().String("dist-path", "dist", "dist output path (react/vite/vue/nextjs-export)")
	cmd.Flags().String("jar-output", "/build/libs/*.jar", "JAR output path (gradle/maven)")
	cmd.Flags().String("binary-name", "app", "binary name (go/rust)")
	cmd.Flags().String("dockerfile", "./Dockerfile", "Dockerfile path (docker)")
	cmd.Flags().String("context", ".", "Docker build context (docker)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would happen")
	return cmd
}

func buildAppBody(name, buildType, owner, repo, branch, installID string, port int, routes []string, cmd *cobra.Command) map[string]any {
	github := map[string]any{
		"owner":          owner,
		"repo":           repo,
		"branch":         branch,
		"installationId": installID,
	}

	var build map[string]any
	switch buildType {
	case "gradle":
		jv, _ := cmd.Flags().GetString("java-version")
		bc, _ := cmd.Flags().GetString("build-command")
		jo, _ := cmd.Flags().GetString("jar-output")
		if bc == "" {
			bc = "./gradlew bootJar -x test"
		}
		build = map[string]any{"gradle": map[string]any{"javaVersion": jv, "buildCommand": bc, "jarOutputPath": jo}}
	case "nodejs":
		nv, _ := cmd.Flags().GetString("node-version")
		bc, _ := cmd.Flags().GetString("build-command")
		sc, _ := cmd.Flags().GetString("start-command")
		if bc == "" {
			bc = "npm install"
		}
		if sc == "" {
			sc = "npm start"
		}
		build = map[string]any{"nodejs": map[string]any{"nodeVersion": nv, "buildCommand": bc, "startCommand": sc}}
	case "react":
		nv, _ := cmd.Flags().GetString("node-version")
		bc, _ := cmd.Flags().GetString("build-command")
		dp, _ := cmd.Flags().GetString("dist-path")
		if bc == "" {
			bc = "npm run build"
		}
		build = map[string]any{"react": map[string]any{"nodeVersion": nv, "buildCommand": bc, "distPath": dp}}
	case "vite":
		nv, _ := cmd.Flags().GetString("node-version")
		bc, _ := cmd.Flags().GetString("build-command")
		dp, _ := cmd.Flags().GetString("dist-path")
		if bc == "" {
			bc = "npm run build"
		}
		build = map[string]any{"vite": map[string]any{"nodeVersion": nv, "buildCommand": bc, "distPath": dp}}
	case "go":
		gv, _ := cmd.Flags().GetString("go-version")
		bc, _ := cmd.Flags().GetString("build-command")
		bn, _ := cmd.Flags().GetString("binary-name")
		if bc == "" {
			bc = "go build -o " + bn + " ."
		}
		build = map[string]any{"go": map[string]any{"goVersion": gv, "buildCommand": bc, "binaryName": bn}}
	case "docker":
		df, _ := cmd.Flags().GetString("dockerfile")
		ctx, _ := cmd.Flags().GetString("context")
		build = map[string]any{"docker": map[string]any{"dockerfilePath": df, "contextPath": ctx}}
	default:
		build = map[string]any{"docker": map[string]any{"dockerfilePath": "./Dockerfile", "contextPath": "."}}
	}

	body := map[string]any{
		"name":   name,
		"github": github,
		"build":  build,
	}
	if port > 0 && len(routes) > 0 {
		body["endpoints"] = []map[string]any{
			{"port": port, "routes": routes},
		}
	} else if port > 0 {
		body["endpoints"] = []map[string]any{
			{"port": port},
		}
	}
	return body
}

func newDeleteCmd() *cobra.Command {
	var yes, dryRun bool
	cmd := &cobra.Command{
		Use:   "delete <app>",
		Short: "Delete an application",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			if dryRun {
				output.Info(fmt.Sprintf("[dry-run] would delete app %s from project %s", args[0], project))
				return nil
			}
			if !yes {
				return fmt.Errorf("use --yes to confirm deletion")
			}
			c := api.FromCmd(cmd)
			if err := c.DeleteApp(cmd.Context(), project, args[0]); err != nil {
				return err
			}
			output.Success(fmt.Sprintf("deleted app %s", args[0]))
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm deletion")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would happen")
	return cmd
}
