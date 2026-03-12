package schema

import (
	"encoding/json"
	"os"

	"github.com/spf13/cobra"
)

type CommandSchema struct {
	Command     string            `json:"command"`
	Description string            `json:"description"`
	Args        []ArgSchema       `json:"args,omitempty"`
	Flags       []FlagSchema      `json:"flags,omitempty"`
	Constraints map[string]string `json:"constraints,omitempty"`
	Examples    []string          `json:"examples,omitempty"`
	SubCommands []CommandSchema   `json:"subcommands,omitempty"`
}

type ArgSchema struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Desc     string `json:"description,omitempty"`
}

type FlagSchema struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Default string `json:"default,omitempty"`
	Desc    string `json:"description"`
}

func NewSchemaCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "schema",
		Short: "Show machine-readable schema of all commands (for AI agents)",
		RunE: func(cmd *cobra.Command, args []string) error {
			s := buildSchema()
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(s)
		},
	}
}

func buildSchema() map[string]any {
	return map[string]any{
		"version": "1.0",
		"name":    "xquare",
		"description": "xquare PaaS CLI — manage projects, apps, addons, and environment variables. " +
			"Always run 'xquare link <project>' first to set project context, or use -p flag.",
		"global_flags": []FlagSchema{
			{Name: "project", Type: "string", Desc: "project name (overrides .xquare/config set by 'xquare link')"},
			{Name: "json", Type: "bool", Desc: "output as JSON (machine-readable)"},
			{Name: "jq", Type: "string", Desc: "filter JSON output with jq expression"},
			{Name: "fields", Type: "string", Desc: "comma-separated fields to include in JSON output"},
		},
		"constraints": map[string]string{
			"project_name": "lowercase letters and numbers only, no hyphens, 2-63 chars. Example: myproject, dsm2025",
			"app_name":     "lowercase letters, numbers, hyphens allowed, 2-63 chars, cannot start/end with hyphen. Example: my-api, backend",
			"addon_name":   "same rules as app_name",
			"storage":      "number + unit, must be less than 4Gi. Examples: 1Gi, 500Mi, 2Gi. Default: 2Gi",
			"build_type":   "gradle | nodejs | react | vite | vue | nextjs | nextjs-export | go | rust | maven | django | flask | docker",
			"addon_type":   "mysql | postgresql | redis | mongodb | kafka | rabbitmq | opensearch | elasticsearch | qdrant",
			"endpoint":     "<port> or <port>:<domain1>,<domain2>. Example: 8080 or 8080:api.dsmhs.kr or 8080:api.dsmhs.kr,admin.dsmhs.kr",
			"trigger_paths": "comma-separated glob patterns for CI trigger filtering. Example: src/**,Dockerfile,go.mod",
		},
		"commands": []CommandSchema{
			{
				Command:     "link <project>",
				Description: "Link current directory to a project. Run this first before other commands.",
				Args:        []ArgSchema{{Name: "project", Required: true, Desc: "project name"}},
				Examples:    []string{"xquare link myproject"},
			},
			{
				Command:     "unlink",
				Description: "Remove project link from current directory.",
				Examples:    []string{"xquare unlink"},
			},
			{
				Command:     "whoami",
				Description: "Show currently logged-in user and linked project.",
				Examples:    []string{"xquare whoami", "xquare whoami --json"},
			},
			projectSchema(),
			appSchema(),
			envSchema(),
			addonSchema(),
			{
				Command:     "deploy <app>",
				Description: "Trigger re-deploy with the latest commit. Use --watch to wait until deployment completes.",
				Args:        []ArgSchema{{Name: "app", Required: true}},
				Flags: []FlagSchema{
					{Name: "watch", Type: "bool", Desc: "wait until deployment is fully running"},
					{Name: "dry-run", Type: "bool", Desc: "show what would happen"},
				},
				Examples: []string{
					"xquare deploy my-api",
					"xquare deploy my-api --watch",
				},
			},
			{
				Command:     "logs <app>",
				Description: "Stream application runtime logs. Use --build for CI build logs.",
				Args:        []ArgSchema{{Name: "app", Required: true}},
				Flags: []FlagSchema{
					{Name: "follow", Type: "bool", Desc: "follow log output (like tail -f)"},
					{Name: "tail", Type: "int", Default: "100", Desc: "number of lines from end"},
					{Name: "since", Type: "string", Desc: "show logs since duration, e.g. 1h, 30m"},
					{Name: "build", Type: "bool", Desc: "show CI build logs instead of runtime logs"},
					{Name: "build-id", Type: "string", Default: "latest", Desc: "specific build ID (with --build)"},
				},
				Examples: []string{
					"xquare logs my-api",
					"xquare logs my-api -f",
					"xquare logs my-api --build",
				},
			},
		},
	}
}

func projectSchema() CommandSchema {
	return CommandSchema{
		Command:     "project",
		Description: "Manage projects. Project names: lowercase letters and numbers only, no hyphens.",
		Constraints: map[string]string{
			"name": "^[a-z0-9]{2,63}$ — no hyphens allowed",
		},
		SubCommands: []CommandSchema{
			{
				Command:     "project list",
				Description: "List all projects you have access to. (* = currently linked)",
				Examples:    []string{"xquare project list", "xquare project list --json"},
			},
			{
				Command:     "project get <project>",
				Description: "Show project details including all apps and addons.",
				Args:        []ArgSchema{{Name: "project", Required: true}},
				Examples:    []string{"xquare project get myproject"},
			},
			{
				Command:     "project create <name>",
				Description: "Create a new project. Name must be lowercase letters and numbers only, no hyphens, 2-63 chars.",
				Args:        []ArgSchema{{Name: "name", Required: true, Desc: "project name, e.g. myproject"}},
				Flags:       []FlagSchema{{Name: "dry-run", Type: "bool", Desc: "preview without creating"}},
				Constraints: map[string]string{"name": "^[a-z0-9]{2,63}$ — no hyphens"},
				Examples:    []string{"xquare project create myproject", "xquare project create dsm2025"},
			},
			{
				Command:     "project delete <project>",
				Description: "Delete a project and all its apps/addons. Requires --yes to confirm.",
				Args:        []ArgSchema{{Name: "project", Required: true}},
				Flags: []FlagSchema{
					{Name: "yes", Type: "bool", Desc: "confirm deletion (required)"},
					{Name: "dry-run", Type: "bool", Desc: "preview without deleting"},
				},
				Examples: []string{"xquare project delete myproject --yes"},
			},
			{
				Command:     "project members",
				Description: "List project members.",
				Examples:    []string{"xquare project members", "xquare project members --json"},
			},
			{
				Command:     "project members add <github-username>",
				Description: "Add a member to the project by GitHub username.",
				Args:        []ArgSchema{{Name: "github-username", Required: true}},
				Examples:    []string{"xquare project members add johndoe"},
			},
			{
				Command:     "project members remove <github-username>",
				Description: "Remove a member from the project. Requires --yes.",
				Args:        []ArgSchema{{Name: "github-username", Required: true}},
				Flags:       []FlagSchema{{Name: "yes", Type: "bool", Desc: "confirm removal (required)"}},
				Examples:    []string{"xquare project members remove johndoe --yes"},
			},
		},
	}
}

func appSchema() CommandSchema {
	return CommandSchema{
		Command:     "app",
		Description: "Manage applications. App names: lowercase letters, numbers, hyphens (2-63 chars).",
		Constraints: map[string]string{
			"name":          "^[a-z0-9][a-z0-9-]{0,61}[a-z0-9]$ — lowercase, hyphens ok, 2-63 chars",
			"build_type":    "gradle | nodejs | react | vite | vue | nextjs | nextjs-export | go | rust | maven | django | flask | docker",
			"endpoint":      "<port> or <port>:<route1>,<route2> — e.g. 8080 or 8080:api.dsmhs.kr",
			"trigger_paths": "glob patterns — e.g. src/**,Dockerfile,go.mod (only trigger CI when matched)",
		},
		SubCommands: []CommandSchema{
			{
				Command:     "app list",
				Description: "List all applications in the project.",
				Flags: []FlagSchema{
					{Name: "status", Type: "bool", Desc: "include K8s deployment status (slower)"},
				},
				Examples: []string{"xquare app list", "xquare app list -s --json"},
			},
			{
				Command:     "app get <app>",
				Description: "Show application configuration (build type, endpoints, GitHub info).",
				Args:        []ArgSchema{{Name: "app", Required: true}},
				Examples:    []string{"xquare app get my-api"},
			},
			{
				Command:     "app status <app>",
				Description: "Show deployment status: running/pending/failed/stopped/not_deployed, instance count, current image hash.",
				Args:        []ArgSchema{{Name: "app", Required: true}},
				Examples:    []string{"xquare app status my-api", "xquare app status my-api --json"},
			},
			{
				Command:     "app create <app>",
				Description: "Create a new application. GitHub owner/repo are auto-detected from git remote if not specified.",
				Args:        []ArgSchema{{Name: "app", Required: true, Desc: "app name, lowercase+hyphens, 2-63 chars"}},
				Flags: []FlagSchema{
					{Name: "build-type", Type: "string", Default: "docker", Desc: "gradle|nodejs|react|vite|vue|nextjs|nextjs-export|go|rust|maven|django|flask|docker"},
					{Name: "endpoint", Type: "stringArray", Desc: "repeatable: 8080 or 8080:api.dsmhs.kr or 8080:a.kr,b.kr"},
					{Name: "owner", Type: "string", Desc: "GitHub org/user (auto-detected from git remote)"},
					{Name: "repo", Type: "string", Desc: "GitHub repo name (auto-detected from git remote)"},
					{Name: "branch", Type: "string", Default: "main", Desc: "GitHub branch"},
					{Name: "trigger-paths", Type: "strings", Desc: "CI trigger path filters, e.g. src/**,Dockerfile"},
					{Name: "java-version", Type: "string", Default: "17", Desc: "for gradle/maven"},
					{Name: "node-version", Type: "string", Default: "20", Desc: "for nodejs/react/vite/vue/nextjs"},
					{Name: "go-version", Type: "string", Default: "1.23", Desc: "for go"},
					{Name: "rust-version", Type: "string", Default: "1.75", Desc: "for rust"},
					{Name: "python-version", Type: "string", Default: "3.11", Desc: "for django/flask"},
					{Name: "build-command", Type: "string", Desc: "override default build command"},
					{Name: "start-command", Type: "string", Desc: "override start command (nodejs/nextjs/django/flask)"},
					{Name: "dist-path", Type: "string", Desc: "frontend dist path (react:/build vite:/dist nextjs-export:/out)"},
					{Name: "jar-output", Type: "string", Default: "/build/libs/*.jar", Desc: "JAR path (gradle/maven)"},
					{Name: "binary-name", Type: "string", Default: "app", Desc: "binary name (go/rust)"},
					{Name: "dockerfile", Type: "string", Default: "./Dockerfile", Desc: "Dockerfile path (docker)"},
					{Name: "context", Type: "string", Default: ".", Desc: "Docker build context (docker)"},
					{Name: "dry-run", Type: "bool", Desc: "preview without creating"},
				},
				Examples: []string{
					"xquare app create my-api --build-type gradle --endpoint 8080:api.dsmhs.kr",
					"xquare app create my-api --build-type go --endpoint 8080",
					"xquare app create frontend --build-type react --endpoint 8080:app.dsmhs.kr",
					"xquare app create backend --build-type nodejs --endpoint 8080 --trigger-paths src/**,package.json",
				},
			},
			{
				Command:     "app update <app>",
				Description: "Update app configuration. Only specified flags are changed.",
				Args:        []ArgSchema{{Name: "app", Required: true}},
				Flags: []FlagSchema{
					{Name: "build-type", Type: "string", Desc: "change build type"},
					{Name: "endpoint", Type: "stringArray", Desc: "replace all endpoints"},
					{Name: "branch", Type: "string", Desc: "change GitHub branch"},
					{Name: "trigger-paths", Type: "strings", Desc: "replace CI trigger paths"},
					{Name: "dry-run", Type: "bool", Desc: "preview without updating"},
				},
				Examples: []string{
					"xquare app update my-api --branch develop",
					"xquare app update my-api --endpoint 8080:newdomain.dsmhs.kr",
				},
			},
			{
				Command:     "app delete <app>",
				Description: "Delete an application. Requires --yes to confirm.",
				Args:        []ArgSchema{{Name: "app", Required: true}},
				Flags: []FlagSchema{
					{Name: "yes", Type: "bool", Desc: "confirm deletion (required)"},
					{Name: "dry-run", Type: "bool", Desc: "preview"},
				},
				Examples: []string{"xquare app delete my-api --yes"},
			},
			{
				Command:     "app tunnel <app>",
				Description: "Open a local port tunnel to an app's service port via wstunnel.",
				Args:        []ArgSchema{{Name: "app", Required: true}},
				Flags: []FlagSchema{
					{Name: "port", Type: "int", Desc: "target app port (required if multiple endpoints)"},
					{Name: "local-port", Type: "int", Desc: "local port to bind (defaults to service port)"},
				},
				Examples: []string{
					"xquare app tunnel my-api",
					"xquare app tunnel my-api --port 9090",
				},
			},
		},
	}
}

func envSchema() CommandSchema {
	return CommandSchema{
		Command:     "env",
		Description: "Manage application environment variables stored in Vault.",
		SubCommands: []CommandSchema{
			{
				Command:     "env get <app>",
				Description: "Show all environment variables for an app.",
				Args:        []ArgSchema{{Name: "app", Required: true}},
				Examples:    []string{"xquare env get my-api", "xquare env get my-api --json"},
			},
			{
				Command:     "env set <app> KEY=VALUE ...",
				Description: "Set environment variables (merges with existing by default). Use --replace --yes to overwrite all.",
				Args: []ArgSchema{
					{Name: "app", Required: true},
					{Name: "KEY=VALUE", Required: true, Desc: "one or more key=value pairs"},
				},
				Flags: []FlagSchema{
					{Name: "replace", Type: "bool", Desc: "replace ALL existing vars (requires --yes)"},
					{Name: "yes", Type: "bool", Desc: "confirm --replace operation"},
					{Name: "dry-run", Type: "bool", Desc: "preview without setting"},
				},
				Examples: []string{
					"xquare env set my-api DB_HOST=localhost DB_PORT=5432",
					"xquare env set my-api --replace --yes KEY=val",
				},
			},
			{
				Command:     "env delete <app> KEY [KEY...]",
				Description: "Delete one or more environment variables.",
				Args: []ArgSchema{
					{Name: "app", Required: true},
					{Name: "KEY", Required: true, Desc: "one or more key names to delete"},
				},
				Flags:    []FlagSchema{{Name: "dry-run", Type: "bool", Desc: "preview"}},
				Examples: []string{"xquare env delete my-api OLD_KEY ANOTHER_KEY"},
			},
			{
				Command:     "env pull <app>",
				Description: "Download env vars to a .env file.",
				Args:        []ArgSchema{{Name: "app", Required: true}},
				Flags:       []FlagSchema{{Name: "output", Type: "string", Default: ".env", Desc: "output file path (- for stdout)"}},
				Examples:    []string{"xquare env pull my-api", "xquare env pull my-api -o prod.env"},
			},
			{
				Command:     "env push <app>",
				Description: "Upload env vars from a .env file.",
				Args:        []ArgSchema{{Name: "app", Required: true}},
				Flags: []FlagSchema{
					{Name: "file", Type: "string", Default: ".env", Desc: "input .env file path"},
					{Name: "replace", Type: "bool", Desc: "replace ALL existing vars (requires --yes)"},
					{Name: "yes", Type: "bool", Desc: "confirm --replace"},
					{Name: "dry-run", Type: "bool", Desc: "preview"},
				},
				Examples: []string{"xquare env push my-api", "xquare env push my-api -f prod.env"},
			},
		},
	}
}

func addonSchema() CommandSchema {
	return CommandSchema{
		Command:     "addon",
		Description: "Manage database/cache addons (StatefulSets with persistent storage).",
		Constraints: map[string]string{
			"type":    "mysql | postgresql | redis | mongodb | kafka | rabbitmq | opensearch | elasticsearch | qdrant",
			"storage": "number + unit, must be less than 4Gi. Examples: 1Gi, 500Mi, 2Gi. Default: 2Gi",
		},
		SubCommands: []CommandSchema{
			{
				Command:     "addon list",
				Description: "List addons in the project with provisioning status.",
				Examples:    []string{"xquare addon list", "xquare addon list --json"},
			},
			{
				Command:     "addon create <name> <type>",
				Description: "Provision a new database/cache addon. Takes 1-2 minutes to become ready.",
				Args: []ArgSchema{
					{Name: "name", Required: true, Desc: "addon name"},
					{Name: "type", Required: true, Desc: "mysql|postgresql|redis|mongodb|kafka|rabbitmq|opensearch|elasticsearch|qdrant"},
				},
				Flags: []FlagSchema{
					{Name: "storage", Type: "string", Default: "2Gi", Desc: "storage size, must be less than 4Gi (e.g. 1Gi, 500Mi, 2Gi)"},
					{Name: "bootstrap", Type: "string", Desc: "bootstrap SQL or script to run on first start"},
					{Name: "dry-run", Type: "bool", Desc: "preview without creating"},
				},
				Constraints: map[string]string{"storage": "< 4Gi required"},
				Examples: []string{
					"xquare addon create mydb mysql",
					"xquare addon create mydb postgresql --storage 2Gi",
					"xquare addon create cache redis --storage 500Mi",
				},
			},
			{
				Command:     "addon delete <name>",
				Description: "Delete an addon and its persistent storage. Requires --yes.",
				Args:        []ArgSchema{{Name: "name", Required: true}},
				Flags: []FlagSchema{
					{Name: "yes", Type: "bool", Desc: "confirm deletion (required)"},
					{Name: "dry-run", Type: "bool", Desc: "preview"},
				},
				Examples: []string{"xquare addon delete mydb --yes"},
			},
			{
				Command:     "addon get <name>",
				Description: "Show connection info for an addon (host, port, password). Password is the wstunnel access key.",
				Args:        []ArgSchema{{Name: "name", Required: true}},
				Examples:    []string{"xquare addon get mydb", "xquare addon get mydb --json"},
			},
			{
				Command:     "addon connect <name>",
				Description: "Open an interactive DB session. Starts tunnel automatically then launches native client (mysql/psql/redis-cli/mongosh).",
				Args:        []ArgSchema{{Name: "name", Required: true}},
				Flags:       []FlagSchema{{Name: "local-port", Type: "int", Desc: "local port (defaults to service port)"}},
				Examples:    []string{"xquare addon connect mydb", "xquare addon connect mydb --local-port 13306"},
			},
			{
				Command:     "addon tunnel <name>",
				Description: "Open local port forwarding to an addon without launching a client. Use --print-url to get connection string only.",
				Args:        []ArgSchema{{Name: "name", Required: true}},
				Flags: []FlagSchema{
					{Name: "local-port", Type: "int", Desc: "local port (defaults to service port)"},
					{Name: "print-url", Type: "bool", Desc: "print connection string and exit (non-interactive)"},
				},
				Examples: []string{
					"xquare addon tunnel mydb",
					"xquare addon tunnel mydb --print-url",
				},
			},
		},
	}
}

