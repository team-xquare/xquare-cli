package app

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/team-xquare/xquare-cli/internal/api"
	"github.com/team-xquare/xquare-cli/internal/output"
	"github.com/team-xquare/xquare-cli/internal/tunnel"
)

var appNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,61}[a-z0-9]$`)

func validateAppName(name string) error {
	if !appNameRe.MatchString(name) {
		return fmt.Errorf("invalid app name %q: must be lowercase letters, numbers, and hyphens (2-63 chars, cannot start or end with hyphen)", name)
	}
	return nil
}

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
		newAppTunnelCmd(),
		newDashboardCmd(),
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
				statusStr, instances := "unknown", "-"
				if r.err == nil && r.status != nil {
					statusStr = fmt.Sprintf("%v", r.status["status"])
					if statusStr != "not_deployed" {
						if sc, ok := r.status["scale"].(map[string]any); ok {
							instances = fmt.Sprintf("%v/%v", sc["running"], sc["desired"])
						}
					}
				}
				rows = append(rows, []string{r.name, statusStr, instances, hash})
			}
			output.Table([]string{"NAME", "STATUS", "INSTANCES", "VERSION"}, rows)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&withStatus, "status", "s", false, "include live status for each app")
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
			if api.IsJSON(cmd) {
				return output.JSON(a)
			}
			rows := [][]string{{"Name", args[0]}}
			if gh, ok := a["github"].(map[string]any); ok {
				rows = append(rows, []string{"GitHub", fmt.Sprintf("%v/%v@%v", gh["owner"], gh["repo"], gh["branch"])})
			}
			// Build type
			if build, ok := a["build"].(map[string]any); ok {
				for buildType := range build {
					rows = append(rows, []string{"Build Type", buildType})
					break
				}
			}
			for _, row := range endpointRows(a) {
				rows = append(rows, row)
			}
			output.Table([]string{"FIELD", "VALUE"}, rows)
			return nil
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
			// Fetch status and app config in parallel to show URL alongside status.
			type result struct {
				status map[string]any
				app    map[string]any
				err    error
			}
			ch := make(chan result, 1)
			go func() {
				a, _ := c.GetApp(cmd.Context(), project, args[0])
				ch <- result{app: a}
			}()
			status, err := c.GetAppStatus(cmd.Context(), project, args[0])
			if err != nil {
				return err
			}
			appCfg := (<-ch).app
			if api.IsJSON(cmd) {
				return output.JSON(status)
			}
			appStatus := fmt.Sprintf("%v", status["status"])
			deployPhase := fmt.Sprintf("%v", status["deployPhase"])
			ciReady := fmt.Sprintf("%v", status["ciReady"]) == "true"
			version := fmt.Sprintf("%v", status["version"])
			if version == "" || version == "<nil>" {
				version = "-"
			} else if len(version) > 8 {
				version = version[:8]
			}
			running, desired := "?", "?"
			if sc, ok := status["scale"].(map[string]any); ok {
				running = fmt.Sprintf("%v", sc["running"])
				desired = fmt.Sprintf("%v", sc["desired"])
			}

			// Human-readable status annotation
			statusDisplay := appStatus
			switch deployPhase {
			case "building":
				statusDisplay = appStatus + "  (building...)"
			case "syncing":
				statusDisplay = appStatus + "  (syncing...)"
			}

			// Instances display
			instancesDisplay := fmt.Sprintf("%s/%s running", running, desired)
			if appStatus == "not_deployed" {
				instancesDisplay = "-"
			}

			ciReadyStr := "ready"
			if !ciReady {
				ciReadyStr = "preparing..."
			}
			rows := [][]string{
				{"Status", statusDisplay},
				{"CI Ready", ciReadyStr},
				{"Instances", instancesDisplay},
				{"Version", version},
			}

			if appStatus == "not_deployed" && !ciReady {
				rows = append(rows, []string{"Hint", "CI/CD pipeline initializing, please wait (~2-3 min)"})
			}

			if lb, ok := status["lastBuild"].(map[string]any); ok && lb != nil {
				lbID := fmt.Sprintf("%v", lb["id"])
				lbStatus := fmt.Sprintf("%v", lb["status"])
				rows = append(rows, []string{"Last Build", fmt.Sprintf("%s  [%s]", lbStatus, lbID)})
			}

			if inst, ok := status["instances"].([]any); ok {
				for i, instance := range inst {
					if p, ok := instance.(map[string]any); ok {
						rows = append(rows, []string{fmt.Sprintf("Instance %d", i+1), fmt.Sprintf("status=%v  restarts=%v", p["status"], p["restarts"])})
					}
				}
			}
			if appCfg != nil {
				for _, row := range endpointRows(appCfg) {
					rows = append(rows, row)
				}
			}
			dashURL := fmt.Sprintf("https://%s-observability-dashboard.dsmhs.kr/d/app-%s", project, args[0])
			rows = append(rows, []string{"Dashboard", dashURL})
			output.Table([]string{"FIELD", "VALUE"}, rows)
			return nil
		},
	}
}

func newCreateCmd() *cobra.Command {
	var dryRun bool
	var wait bool
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
			if err := validateAppName(appName); err != nil {
				return err
			}
			buildType, _ := cmd.Flags().GetString("build-type")
			owner, _ := cmd.Flags().GetString("owner")
			repo, _ := cmd.Flags().GetString("repo")
			branch, _ := cmd.Flags().GetString("branch")
			endpointStrs, _ := cmd.Flags().GetStringArray("endpoint")

			// Validate build-type
			validBuildTypes := map[string]bool{
				"gradle": true, "nodejs": true, "react": true, "vite": true,
				"vue": true, "nextjs": true, "nextjs-export": true,
				"go": true, "rust": true, "maven": true, "django": true,
				"flask": true, "docker": true,
			}
			if !validBuildTypes[buildType] {
				return fmt.Errorf("invalid --build-type %q\n\nSupported types: gradle, nodejs, react, vite, vue, nextjs, nextjs-export, go, rust, maven, django, flask, docker", buildType)
			}

			// Auto-detect owner/repo from git remote if not specified.
			// If --repo contains a slash (e.g. "team-xquare/my-repo"), split it.
			ownerChanged := cmd.Flags().Changed("owner")
			if strings.Contains(repo, "/") {
				parts := strings.SplitN(repo, "/", 2)
				if !ownerChanged {
					owner = parts[0]
				}
				repo = parts[1]
			}
			autoDetected := false
			if repo == "" || !ownerChanged {
				detectedOwner, detectedRepo := detectGitOrigin()
				if repo == "" {
					if detectedRepo != "" {
						repo = detectedRepo
						autoDetected = true
					} else {
						return fmt.Errorf("--repo is required (e.g. --repo my-repo or --repo owner/my-repo)\n\n  xquare app create %s --repo <github-repo-name>", appName)
					}
				}
				if !ownerChanged && detectedOwner != "" {
					owner = detectedOwner
					autoDetected = true
				}
			}
			if owner == "" {
				return fmt.Errorf("--owner is required (e.g. --owner my-github-org)\n\n  xquare app create %s --owner <github-org>", appName)
			}
			if autoDetected {
				output.Info(fmt.Sprintf("detected: %s/%s", owner, repo))
			}

			endpoints, err := parseEndpoints(endpointStrs)
			if err != nil {
				return err
			}
			body := buildAppBody(appName, buildType, owner, repo, branch, endpoints, cmd)
			if triggerPaths, _ := cmd.Flags().GetStringSlice("trigger-paths"); len(triggerPaths) > 0 {
				if gh, ok := body["github"].(map[string]any); ok {
					gh["triggerPaths"] = triggerPaths
				}
			}
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
			for _, ep := range endpoints {
				port := fmt.Sprintf("%v", ep["port"])
				if routes, ok := ep["routes"].([]string); ok && len(routes) > 0 {
					for _, r := range routes {
						output.Info(fmt.Sprintf("  :%s → https://%s", port, r))
					}
				} else {
					output.Info(fmt.Sprintf("  :%s (internal)", port))
				}
			}

			if wait {
				output.Info("initializing CI/CD pipeline... (Ctrl+C to cancel)")
				if err := waitCIReady(cmd, api.FromCmd(cmd), project, appName); err != nil {
					return err
				}
				output.Success("CI/CD pipeline ready — push to GitHub to deploy")
				return nil
			}

			output.Info("")
			output.Info("CI/CD pipeline initializing... (~2-3 min)")
			output.Info("once ready: push to GitHub to trigger auto-deploy")
			output.Info(fmt.Sprintf("  xquare app status %s   # check readiness", appName))
			output.Info(fmt.Sprintf("  xquare env set %s KEY=value   # set environment variables", appName))
			return nil
		},
	}
	cmd.Flags().String("build-type", "docker", "build type: gradle|nodejs|react|vite|vue|nextjs|nextjs-export|go|rust|maven|django|flask|docker")
	cmd.Flags().String("owner", "", "GitHub owner (auto-detected from git remote)")
	cmd.Flags().String("repo", "", "GitHub repository name")
	cmd.Flags().String("branch", "main", "GitHub branch")
	cmd.Flags().StringArray("endpoint", []string{}, "expose port with optional domain: <port>[:<hostname>] (e.g. --endpoint 8080:myapp.dsmhs.kr). The hostname becomes the public URL (https://hostname).")
	cmd.Flags().StringSlice("trigger-paths", []string{}, "only trigger CI when these paths change (e.g. src/**,Dockerfile)")
	cmd.Flags().String("java-version", "17", "Java version")
	cmd.Flags().String("node-version", "20", "Node.js version")
	cmd.Flags().String("go-version", "1.23", "Go version")
	cmd.Flags().String("rust-version", "1.75", "Rust version")
	cmd.Flags().String("python-version", "3.11", "Python version")
	cmd.Flags().String("build-command", "", "build command override")
	cmd.Flags().String("start-command", "", "start command")
	cmd.Flags().String("dist-path", "dist", "dist output path (react: /build, vite/vue: /dist, nextjs-export: /out)")
	cmd.Flags().String("jar-output", "/build/libs/*.jar", "JAR output path")
	cmd.Flags().String("binary-name", "app", "binary name")
	cmd.Flags().String("dockerfile", "./Dockerfile", "Dockerfile path")
	cmd.Flags().String("context", ".", "Docker build context")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would happen without creating")
	cmd.Flags().BoolVar(&wait, "wait", false, "wait until CI/CD pipeline is ready (~2-3 min)")
	return cmd
}

// waitCIReady polls app status until ciReady=true, up to 5 minutes.
func waitCIReady(cmd *cobra.Command, c *api.Client, project, app string) error {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	timeout := time.After(5 * time.Minute)
	for {
		select {
		case <-cmd.Context().Done():
			return nil
		case <-timeout:
			return fmt.Errorf("timeout: CI/CD pipeline not ready after 5 minutes\n\n  xquare app status %s   # check status", app)
		case <-ticker.C:
			status, err := c.GetAppStatus(cmd.Context(), project, app)
			if err != nil {
				continue
			}
			if fmt.Sprintf("%v", status["ciReady"]) == "true" {
				return nil
			}
		}
	}
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
			if cmd.Flags().Changed("owner") || cmd.Flags().Changed("repo") || cmd.Flags().Changed("branch") {
				// start from existing github fields
				gh := map[string]any{}
				if existingGH, ok := existing["github"].(map[string]any); ok {
					for k, v := range existingGH {
						gh[k] = v
					}
				}
				if cmd.Flags().Changed("owner") {
					owner, _ := cmd.Flags().GetString("owner")
					gh["owner"] = owner
				}
				if cmd.Flags().Changed("repo") {
					repo, _ := cmd.Flags().GetString("repo")
					gh["repo"] = repo
				}
				if cmd.Flags().Changed("branch") {
					branch, _ := cmd.Flags().GetString("branch")
					gh["branch"] = branch
				}
				body["github"] = gh
			}
			if cmd.Flags().Changed("build-type") {
				buildType, _ := cmd.Flags().GetString("build-type")
				validBuildTypes := map[string]bool{
					"gradle": true, "nodejs": true, "react": true, "vite": true,
					"vue": true, "nextjs": true, "nextjs-export": true,
					"go": true, "rust": true, "maven": true, "django": true,
					"flask": true, "docker": true,
				}
				if !validBuildTypes[buildType] {
					return fmt.Errorf("invalid --build-type %q\n\nSupported types: gradle, nodejs, react, vite, vue, nextjs, nextjs-export, go, rust, maven, django, flask, docker", buildType)
				}
				body["build"] = buildBody(buildType, cmd)
			}
			if cmd.Flags().Changed("endpoint") {
				endpointStrs, _ := cmd.Flags().GetStringArray("endpoint")
				endpoints, err := parseEndpoints(endpointStrs)
				if err != nil {
					return err
				}
				body["endpoints"] = endpoints
			}
			if cmd.Flags().Changed("trigger-paths") {
				triggerPaths, _ := cmd.Flags().GetStringSlice("trigger-paths")
				if gh, ok := body["github"].(map[string]any); ok {
					gh["triggerPaths"] = triggerPaths
				}
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
	cmd.Flags().StringArray("endpoint", []string{}, "service endpoint: <port>[:<route1>,<route2>] (repeatable)")
	cmd.Flags().StringSlice("trigger-paths", []string{}, "only trigger CI when these paths change")
	cmd.Flags().String("java-version", "17", "Java version")
	cmd.Flags().String("node-version", "20", "Node.js version")
	cmd.Flags().String("go-version", "1.23", "Go version")
	cmd.Flags().String("rust-version", "1.75", "Rust version")
	cmd.Flags().String("python-version", "3.11", "Python version")
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
		if !cmd.Flags().Changed("dist-path") {
			dp = "/build"
		}
		return map[string]any{"react": map[string]any{"nodeVersion": nv, "buildCommand": bc, "distPath": dp}}
	case "vite":
		nv, _ := cmd.Flags().GetString("node-version")
		bc, _ := cmd.Flags().GetString("build-command")
		dp, _ := cmd.Flags().GetString("dist-path")
		if bc == "" {
			bc = "npm run build"
		}
		if !cmd.Flags().Changed("dist-path") {
			dp = "/dist"
		}
		return map[string]any{"vite": map[string]any{"nodeVersion": nv, "buildCommand": bc, "distPath": dp}}
	case "vue":
		nv, _ := cmd.Flags().GetString("node-version")
		bc, _ := cmd.Flags().GetString("build-command")
		dp, _ := cmd.Flags().GetString("dist-path")
		if bc == "" {
			bc = "npm run build"
		}
		if !cmd.Flags().Changed("dist-path") {
			dp = "/dist"
		}
		return map[string]any{"vue": map[string]any{"nodeVersion": nv, "buildCommand": bc, "distPath": dp}}
	case "nextjs":
		nv, _ := cmd.Flags().GetString("node-version")
		bc, _ := cmd.Flags().GetString("build-command")
		sc, _ := cmd.Flags().GetString("start-command")
		if bc == "" {
			bc = "npm ci && npm run build"
		}
		if sc == "" {
			sc = "npm start"
		}
		return map[string]any{"nextjs": map[string]any{"nodeVersion": nv, "buildCommand": bc, "startCommand": sc}}
	case "nextjs-export":
		nv, _ := cmd.Flags().GetString("node-version")
		bc, _ := cmd.Flags().GetString("build-command")
		dp, _ := cmd.Flags().GetString("dist-path")
		if bc == "" {
			bc = "npm ci && npm run build"
		}
		if !cmd.Flags().Changed("dist-path") {
			dp = "/out"
		}
		return map[string]any{"nextjs-export": map[string]any{"nodeVersion": nv, "buildCommand": bc, "distPath": dp}}
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
		bc, _ := cmd.Flags().GetString("build-command")
		sc, _ := cmd.Flags().GetString("start-command")
		if sc == "" {
			sc = "gunicorn app.wsgi:application"
		}
		return map[string]any{"django": map[string]any{"pythonVersion": pv, "buildCommand": bc, "startCommand": sc}}
	case "flask":
		pv, _ := cmd.Flags().GetString("python-version")
		bc, _ := cmd.Flags().GetString("build-command")
		sc, _ := cmd.Flags().GetString("start-command")
		if sc == "" {
			sc = "gunicorn app:app"
		}
		return map[string]any{"flask": map[string]any{"pythonVersion": pv, "buildCommand": bc, "startCommand": sc}}
	case "maven":
		jv, _ := cmd.Flags().GetString("java-version")
		bc, _ := cmd.Flags().GetString("build-command")
		jo, _ := cmd.Flags().GetString("jar-output")
		if bc == "" {
			bc = "mvn package -DskipTests"
		}
		return map[string]any{"maven": map[string]any{"javaVersion": jv, "buildCommand": bc, "jarOutputPath": jo}}
	case "rust":
		rv, _ := cmd.Flags().GetString("rust-version")
		bc, _ := cmd.Flags().GetString("build-command")
		bn, _ := cmd.Flags().GetString("binary-name")
		if bc == "" {
			bc = "cargo build --release"
		}
		return map[string]any{"rust": map[string]any{"rustVersion": rv, "buildCommand": bc, "binaryName": bn}}
	default:
		df, _ := cmd.Flags().GetString("dockerfile")
		ctx, _ := cmd.Flags().GetString("context")
		return map[string]any{"docker": map[string]any{"dockerfilePath": df, "contextPath": ctx}}
	}
}

// detectGitOrigin extracts (owner, repo) from git remote origin URL.
// Returns empty strings if not in a git repo or no remote.
func detectGitOrigin() (owner, repo string) {
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return "", ""
	}
	rawURL := strings.TrimSpace(string(out))
	// Strip credentials from HTTPS URLs (https://token@github.com/owner/repo.git)
	if idx := strings.LastIndex(rawURL, "@"); idx != -1 {
		rawURL = "https://" + rawURL[idx+1:]
	}
	// Normalize git@github.com:owner/repo.git → owner/repo
	rawURL = strings.TrimSuffix(rawURL, ".git")
	rawURL = strings.Replace(rawURL, ":", "/", 1) // git@ format
	parts := strings.Split(rawURL, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2], parts[len(parts)-1]
	}
	return "", ""
}

// xquare app tunnel <app> [--port <port>] — tunnel to an app's service port
func newAppTunnelCmd() *cobra.Command {
	var localPort int
	var targetPort int
	var printURL bool

	cmd := &cobra.Command{
		Use:   "tunnel <app>",
		Short: "Open a local port tunnel to an app's service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := api.FromCmd(cmd)
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			appName := args[0]

			info, err := c.GetAppTunnel(cmd.Context(), project, appName)
			if err != nil {
				return fmt.Errorf("get tunnel info: %w", err)
			}

			tunnelHost := fmt.Sprintf("%v", info["host"])
			password := fmt.Sprintf("%v", info["password"])

			// Resolve which port to tunnel
			var ports []int
			if rawPorts, ok := info["ports"].([]any); ok {
				for _, p := range rawPorts {
					if f, ok := p.(float64); ok {
						ports = append(ports, int(f))
					}
				}
			}

			if len(ports) == 0 {
				return fmt.Errorf("app %q has no service ports configured", appName)
			}

			tunnelPort := ports[0]
			if targetPort > 0 {
				found := false
				for _, p := range ports {
					if p == targetPort {
						found = true
						tunnelPort = targetPort
						break
					}
				}
				if !found {
					available := make([]string, len(ports))
					for i, p := range ports {
						available[i] = fmt.Sprintf("%d", p)
					}
					return fmt.Errorf("port %d not found in app endpoints\n\nAvailable ports: %s", targetPort, strings.Join(available, ", "))
				}
			} else if len(ports) > 1 {
				available := make([]string, len(ports))
				for i, p := range ports {
					available[i] = fmt.Sprintf("%d", p)
				}
				output.Info(fmt.Sprintf("multiple ports available: %s — using %d (use --port to specify)", strings.Join(available, ", "), tunnelPort))
			}

			if localPort == 0 {
				localPort = appFreePort(tunnelPort)
			}

			if printURL {
				fmt.Fprintf(os.Stdout, "http://127.0.0.1:%d\n", localPort)
				return nil
			}

			wstunnelBin, cleanupBin, appTunnelErr := appResolveBinary()
			if cleanupBin != nil {
				defer cleanupBin()
			}
			if appTunnelErr != nil {
				return appTunnelErr
			}

			output.Info(fmt.Sprintf("tunneling localhost:%d → %s:%s:%d", localPort, tunnelHost, appName, tunnelPort))
			output.Info("press Ctrl+C to stop")

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

			localArg := fmt.Sprintf("tcp://0.0.0.0:%d:%s:%d", localPort, appName, tunnelPort)
			proc := exec.Command(wstunnelBin, "client",
				"-L", localArg,
				"--log-lvl", "OFF",
				fmt.Sprintf("wss://%s", tunnelHost),
			)
			proc.Env = append(os.Environ(), "WSTUNNEL_HTTP_UPGRADE_PATH_PREFIX="+password)
			proc.Stdout = os.Stdout
			proc.Stderr = os.Stderr
			if err := proc.Start(); err != nil {
				return fmt.Errorf("start wstunnel: %w", err)
			}

			go func() {
				<-sigCh
				output.Info("\ntunnel closed")
				proc.Process.Kill()
			}()

			return proc.Wait()
		},
	}
	cmd.Flags().IntVar(&localPort, "local-port", 0, "local port (defaults to service port)")
	cmd.Flags().IntVar(&targetPort, "port", 0, "app port to tunnel (required if multiple ports)")
	cmd.Flags().BoolVar(&printURL, "print-url", false, "print tunnel URL and exit (non-interactive)")
	return cmd
}

func appResolveBinary() (binPath string, cleanup func(), err error) {
	return tunnel.ExtractWstunnel()
}

func appFreePort(preferred int) int {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", preferred), 200*time.Millisecond)
	if err != nil {
		return preferred
	}
	conn.Close()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return preferred
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// parseEndpoints parses --endpoint flags of the form "<port>[:<route1>,<route2>...]"
func parseEndpoints(strs []string) ([]map[string]any, error) {
	if len(strs) == 0 {
		return nil, nil
	}
	var endpoints []map[string]any
	for _, s := range strs {
		parts := strings.SplitN(s, ":", 2)
		port, err := strconv.Atoi(parts[0])
		if err != nil || port <= 0 || port > 65535 {
			return nil, fmt.Errorf("invalid endpoint %q: port must be a number between 1-65535", s)
		}
		ep := map[string]any{"port": port}
		if len(parts) == 2 && parts[1] != "" {
			routes := strings.Split(parts[1], ",")
			var filtered []string
			for _, r := range routes {
				r = strings.TrimSpace(r)
				if r == "" {
					continue
				}
				if err := validateRouteHost(r); err != nil {
					return nil, fmt.Errorf("invalid route %q: %w", r, err)
				}
				filtered = append(filtered, r)
			}
			if len(filtered) > 0 {
				ep["routes"] = filtered
			}
		}
		endpoints = append(endpoints, ep)
	}
	return endpoints, nil
}

// validateRouteHost checks that a route hostname contains only printable ASCII
// characters and is within length limits. The server enforces full DNS validation.
func validateRouteHost(host string) error {
	if len(host) > 253 {
		return fmt.Errorf("hostname exceeds 253 characters")
	}
	for _, b := range []byte(host) {
		if b == 0 {
			return fmt.Errorf("hostname contains null byte")
		}
		if b < 0x20 || b > 0x7e {
			return fmt.Errorf("hostname contains non-printable or non-ASCII character")
		}
	}
	return nil
}

func newDashboardCmd() *cobra.Command {
	var withCreds bool
	cmd := &cobra.Command{
		Use:   "dashboard <app>",
		Short: "Show Grafana dashboard URL for an app",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := api.FromCmd(cmd)
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			appName := args[0]
			dashURL := fmt.Sprintf("https://%s-observability-dashboard.dsmhs.kr/d/app-%s", project, appName)

			if withCreds {
				info, err := c.GetDashboard(cmd.Context(), project)
				if err != nil {
					return err
				}
				password := "(provisioning...)"
				if info.Password != nil {
					password = *info.Password
				}
				if api.IsJSON(cmd) {
					return output.JSON(map[string]any{
						"url": dashURL, "app": appName, "project": project,
						"username": info.Username, "password": password,
					})
				}
				output.Table([]string{"FIELD", "VALUE"}, [][]string{
					{"Dashboard URL", dashURL},
					{"Username", info.Username},
					{"Password", password},
				})
				return nil
			}

			if api.IsJSON(cmd) {
				return output.JSON(map[string]string{"url": dashURL, "app": appName, "project": project})
			}
			fmt.Fprintln(os.Stdout, dashURL)
			output.Info("Tip: use --credentials to also show Grafana login info")
			return nil
		},
	}
	cmd.Flags().BoolVar(&withCreds, "credentials", false, "also show Grafana username and password")
	return cmd
}

func buildAppBody(name, buildType, owner, repo, branch string, endpoints []map[string]any, cmd *cobra.Command) map[string]any {
	body := map[string]any{
		"name": name,
		"github": map[string]any{
			"owner":  owner,
			"repo":   repo,
			"branch": branch,
		},
		"build": buildBody(buildType, cmd),
	}
	if len(endpoints) > 0 {
		body["endpoints"] = endpoints
	}
	return body
}

// endpointRows formats an app's endpoints as table rows for display.
// Each endpoint becomes one row: ":port → https://route1, https://route2"
// If no routes are configured, it shows ":port (internal)".
func endpointRows(a map[string]any) [][]string {
	eps, ok := a["endpoints"].([]any)
	if !ok || len(eps) == 0 {
		return nil
	}
	var rows [][]string
	for _, e := range eps {
		ep, ok := e.(map[string]any)
		if !ok {
			continue
		}
		port := fmt.Sprintf("%v", ep["port"])
		if routes, ok := ep["routes"].([]any); ok && len(routes) > 0 {
			var urls []string
			for _, r := range routes {
				urls = append(urls, "https://"+fmt.Sprintf("%v", r))
			}
			rows = append(rows, []string{"Endpoint", ":" + port + " → " + strings.Join(urls, ", ")})
		} else {
			rows = append(rows, []string{"Endpoint", ":" + port + " (internal)"})
		}
	}
	return rows
}
