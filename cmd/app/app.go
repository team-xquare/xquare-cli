package app

import (
	"fmt"
	"sync"

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
		newUpdateCmd(),
		newDeleteCmd(),
	)
	return cmd
}

func newListCmd() *cobra.Command {
	var withStatus bool
	cmd := &cobra.Command{
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
			if !withStatus {
				rows := make([][]string, 0, len(apps))
				for _, a := range apps {
					name := fmt.Sprintf("%v", a["name"])
					github := ""
					if gh, ok := a["github"].(map[string]any); ok {
						github = fmt.Sprintf("%v/%v@%v", gh["owner"], gh["repo"], gh["branch"])
					}
					rows = append(rows, []string{name, github})
				}
				output.Table([]string{"NAME", "GITHUB"}, rows)
				return nil
			}
			// Parallel status fetch
			type result struct {
				name   string
				status map[string]any
				err    error
			}
			results := make([]result, len(apps))
			var wg sync.WaitGroup
			for i, a := range apps {
				wg.Add(1)
				go func(idx int, app map[string]any) {
					defer wg.Done()
					name := fmt.Sprintf("%v", app["name"])
					results[idx].name = name
					st, err := c.GetAppStatus(cmd.Context(), project, name)
					results[idx].status = st
					results[idx].err = err
				}(i, a)
			}
			wg.Wait()
			rows := make([][]string, 0, len(apps))
			for i, r := range results {
				a := apps[i]
				hash := "-"
				if gh, ok := a["github"].(map[string]any); ok {
					h := fmt.Sprintf("%v", gh["hash"])
					if len(h) >= 8 {
						h = h[:8]
					}
					if h != "<nil>" && h != "" {
						hash = h
					}
				}
				statusStr, replicas := "Unknown", "-"
				if r.err == nil && r.status != nil {
					statusStr = fmt.Sprintf("%v", r.status["status"])
					if rep, ok := r.status["replicas"].(map[string]any); ok {
						replicas = fmt.Sprintf("%v/%v", rep["ready"], rep["desired"])
					}
				}
				rows = append(rows, []string{r.name, statusStr, replicas, hash})
			}
			output.Table([]string{"NAME", "STATUS", "REPLICAS", "HASH"}, rows)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&withStatus, "status", "s", false, "include deployment status (parallel K8s queries)")
	return cmd
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
			appStatus := fmt.Sprintf("%v", status["status"])
			hash := fmt.Sprintf("%v", status["hash"])
			if len(hash) > 8 {
				hash = hash[:8]
			}
			ready, desired := "?", "?"
			if rep, ok := status["replicas"].(map[string]any); ok {
				ready = fmt.Sprintf("%v", rep["ready"])
				desired = fmt.Sprintf("%v", rep["desired"])
			}
			rows := [][]string{
				{"Status", appStatus},
				{"Replicas", fmt.Sprintf("%s/%s", ready, desired)},
				{"Hash", hash},
			}
			if pods, ok := status["pods"].([]any); ok {
				for _, pod := range pods {
					if p, ok := pod.(map[string]any); ok {
						rows = append(rows, []string{"Pod", fmt.Sprintf("%v  phase=%v  restarts=%v", p["name"], p["phase"], p["restartCount"])})
					}
				}
			}
			output.Table([]string{"FIELD", "VALUE"}, rows)
			return nil
		},
	}
}

func newCreateCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "create <app>",
		Short: "Create a new application",
		Args:  cobra.ExactArgs(1),
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
			body := buildAppBody(appName, buildType, owner, repo, branch, installID, port, routes, cmd)
			if dryRun {
				output.Info(fmt.Sprintf("[dry-run] would create app %s in project %s", appName, project))
				return output.JSON(body)
			}
			c := api.FromCmd(cmd)
			result, err := c.CreateApp(cmd.Context(), project, body)
			if err != nil {
				return err
			}
			if api.IsJSON(cmd) {
				return output.JSON(result)
			}
			output.Success(fmt.Sprintf("created app %s in project %s", appName, project))
			output.Info(fmt.Sprintf("  push to %s/%s@%s to trigger first build", owner, repo, branch))
			return nil
		},
	}
	cmd.Flags().String("build-type", "docker", "build type: gradle|nodejs|react|vite|go|rust|maven|django|flask|docker")
	cmd.Flags().String("owner", "team-xquare", "GitHub owner")
	cmd.Flags().String("repo", "", "GitHub repository name")
	cmd.Flags().String("branch", "main", "GitHub branch")
	cmd.Flags().String("installation-id", "62433388", "GitHub App installation ID")
	cmd.Flags().Int("port", 8080, "application port")
	cmd.Flags().StringSlice("routes", []string{}, "HTTP routes (e.g. myapp.dsmhs.kr)")
	cmd.Flags().String("java-version", "17", "Java version")
	cmd.Flags().String("node-version", "20", "Node.js version")
	cmd.Flags().String("go-version", "1.21", "Go version")
	cmd.Flags().String("python-version", "3.11", "Python version")
	cmd.Flags().String("build-command", "", "build command override")
	cmd.Flags().String("start-command", "", "start command")
	cmd.Flags().String("dist-path", "dist", "dist output path")
	cmd.Flags().String("jar-output", "/build/libs/*.jar", "JAR output path")
	cmd.Flags().String("binary-name", "app", "binary name")
	cmd.Flags().String("dockerfile", "./Dockerfile", "Dockerfile path")
	cmd.Flags().String("context", ".", "Docker build context")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would happen without creating")
	return cmd
}

func newUpdateCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "update <app>",
		Short: "Update application configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			appName := args[0]
			c := api.FromCmd(cmd)
			existing, err := c.GetApp(cmd.Context(), project, appName)
			if err != nil {
				return err
			}
			body := existing
			body["name"] = appName
			if cmd.Flags().Changed("owner") || cmd.Flags().Changed("repo") ||
				cmd.Flags().Changed("branch") || cmd.Flags().Changed("installation-id") {
				// start from existing github fields
				gh := map[string]any{}
				if existingGH, ok := existing["github"].(map[string]any); ok {
					for k, v := range existingGH {
						gh[k] = v
					}
				}
				if cmd.Flags().Changed("owner") { owner, _ := cmd.Flags().GetString("owner"); gh["owner"] = owner }
				if cmd.Flags().Changed("repo") { repo, _ := cmd.Flags().GetString("repo"); gh["repo"] = repo }
				if cmd.Flags().Changed("branch") { branch, _ := cmd.Flags().GetString("branch"); gh["branch"] = branch }
				if cmd.Flags().Changed("installation-id") { installID, _ := cmd.Flags().GetString("installation-id"); gh["installationId"] = installID }
				body["github"] = gh
			}
			if cmd.Flags().Changed("build-type") {
				buildType, _ := cmd.Flags().GetString("build-type")
				body["build"] = buildBody(buildType, cmd)
			}
			if cmd.Flags().Changed("port") || cmd.Flags().Changed("routes") {
				// start from existing endpoint
				existingPort := 8080
				existingRoutes := []string{}
				if eps, ok := existing["endpoints"].([]any); ok && len(eps) > 0 {
					if ep, ok := eps[0].(map[string]any); ok {
						if p, ok := ep["port"].(float64); ok {
							existingPort = int(p)
						}
						if r, ok := ep["routes"].([]any); ok {
							for _, route := range r {
								existingRoutes = append(existingRoutes, fmt.Sprintf("%v", route))
							}
						}
					}
				}
				port := existingPort
				routes := existingRoutes
				if cmd.Flags().Changed("port") {
					port, _ = cmd.Flags().GetInt("port")
				}
				if cmd.Flags().Changed("routes") {
					routes, _ = cmd.Flags().GetStringSlice("routes")
				}
				body["endpoints"] = []map[string]any{{"port": port, "routes": routes}}
			}
			if dryRun {
				output.Info(fmt.Sprintf("[dry-run] would update app %s in project %s", appName, project))
				return output.JSON(body)
			}
			result, err := c.UpdateApp(cmd.Context(), project, appName, body)
			if err != nil {
				return err
			}
			if api.IsJSON(cmd) {
				return output.JSON(result)
			}
			output.Success(fmt.Sprintf("updated app %s", appName))
			return nil
		},
	}
	cmd.Flags().String("build-type", "", "build type")
	cmd.Flags().String("owner", "", "GitHub owner")
	cmd.Flags().String("repo", "", "GitHub repository")
	cmd.Flags().String("branch", "", "GitHub branch")
	cmd.Flags().String("installation-id", "", "GitHub App installation ID")
	cmd.Flags().Int("port", 0, "application port")
	cmd.Flags().StringSlice("routes", []string{}, "HTTP routes")
	cmd.Flags().String("java-version", "17", "Java version")
	cmd.Flags().String("node-version", "20", "Node.js version")
	cmd.Flags().String("go-version", "1.21", "Go version")
	cmd.Flags().String("build-command", "", "build command")
	cmd.Flags().String("start-command", "", "start command")
	cmd.Flags().String("dist-path", "dist", "dist output path")
	cmd.Flags().String("jar-output", "/build/libs/*.jar", "JAR output path")
	cmd.Flags().String("binary-name", "app", "binary name")
	cmd.Flags().String("dockerfile", "./Dockerfile", "Dockerfile path")
	cmd.Flags().String("context", ".", "Docker build context")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would happen")
	return cmd
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
				return fmt.Errorf("use --yes to confirm deletion of app %s", args[0])
			}
			c := api.FromCmd(cmd)
			if err := c.DeleteApp(cmd.Context(), project, args[0]); err != nil {
				return err
			}
			output.Success(fmt.Sprintf("deleted app %s", args[0]))
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "confirm deletion")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would happen")
	return cmd
}

func buildBody(buildType string, cmd *cobra.Command) map[string]any {
	switch buildType {
	case "gradle":
		jv, _ := cmd.Flags().GetString("java-version")
		bc, _ := cmd.Flags().GetString("build-command")
		jo, _ := cmd.Flags().GetString("jar-output")
		if bc == "" {
			bc = "./gradlew bootJar -x test"
		}
		return map[string]any{"gradle": map[string]any{"javaVersion": jv, "buildCommand": bc, "jarOutputPath": jo}}
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
		return map[string]any{"nodejs": map[string]any{"nodeVersion": nv, "buildCommand": bc, "startCommand": sc}}
	case "react":
		nv, _ := cmd.Flags().GetString("node-version")
		bc, _ := cmd.Flags().GetString("build-command")
		dp, _ := cmd.Flags().GetString("dist-path")
		if bc == "" {
			bc = "npm run build"
		}
		return map[string]any{"react": map[string]any{"nodeVersion": nv, "buildCommand": bc, "distPath": dp}}
	case "vite":
		nv, _ := cmd.Flags().GetString("node-version")
		bc, _ := cmd.Flags().GetString("build-command")
		dp, _ := cmd.Flags().GetString("dist-path")
		if bc == "" {
			bc = "npm run build"
		}
		return map[string]any{"vite": map[string]any{"nodeVersion": nv, "buildCommand": bc, "distPath": dp}}
	case "go":
		gv, _ := cmd.Flags().GetString("go-version")
		bc, _ := cmd.Flags().GetString("build-command")
		bn, _ := cmd.Flags().GetString("binary-name")
		if bc == "" {
			bc = "go build -o " + bn + " ."
		}
		return map[string]any{"go": map[string]any{"goVersion": gv, "buildCommand": bc, "binaryName": bn}}
	case "django":
		pv, _ := cmd.Flags().GetString("python-version")
		sc, _ := cmd.Flags().GetString("start-command")
		if sc == "" {
			sc = "gunicorn app.wsgi:application"
		}
		return map[string]any{"django": map[string]any{"pythonVersion": pv, "startCommand": sc}}
	case "flask":
		pv, _ := cmd.Flags().GetString("python-version")
		sc, _ := cmd.Flags().GetString("start-command")
		if sc == "" {
			sc = "gunicorn app:app"
		}
		return map[string]any{"flask": map[string]any{"pythonVersion": pv, "startCommand": sc}}
	default:
		df, _ := cmd.Flags().GetString("dockerfile")
		ctx, _ := cmd.Flags().GetString("context")
		return map[string]any{"docker": map[string]any{"dockerfilePath": df, "contextPath": ctx}}
	}
}

func buildAppBody(name, buildType, owner, repo, branch, installID string, port int, routes []string, cmd *cobra.Command) map[string]any {
	body := map[string]any{
		"name": name,
		"github": map[string]any{
			"owner":          owner,
			"repo":           repo,
			"branch":         branch,
			"installationId": installID,
		},
		"build": buildBody(buildType, cmd),
	}
	if port > 0 && len(routes) > 0 {
		body["endpoints"] = []map[string]any{{"port": port, "routes": routes}}
	} else if port > 0 {
		body["endpoints"] = []map[string]any{{"port": port}}
	}
	return body
}
