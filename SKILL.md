# xquare CLI — AI Agent Reference (SKILL.md)

> Copy this file into your AI agent's context or system prompt.

## Overview

xquare is a PaaS CLI for DSM student teams. It manages projects, apps, addons (databases/caches),
and environment variables on an on-premises Kubernetes cluster.

## Setup (always do this first)

```bash
xquare login                    # authenticate with GitHub (opens browser)
xquare link <project>           # set default project for current directory
xquare schema                   # see all commands, constraints, valid values
xquare upgrade                  # upgrade CLI to the latest version
```

## Critical Constraints

```
project name:  ^[a-z0-9]{2,63}$     — lowercase letters and numbers ONLY, NO hyphens
app name:      ^[a-z0-9][a-z0-9-]{0,61}[a-z0-9]$  — hyphens allowed
storage:       must be < 4Gi (e.g. 500Mi, 1Gi, 2Gi, 3Gi)
build_type:    gradle | nodejs | react | vite | vue | nextjs | nextjs-export |
               go | rust | maven | django | flask | docker
addon_type:    mysql | postgresql | redis | mongodb | kafka | rabbitmq |
               opensearch | elasticsearch | qdrant
endpoint:      <port> or <port>:<domain1>,<domain2>
               e.g. 8080  or  8080:api.dsmhs.kr  or  8080:api.dsmhs.kr,admin.dsmhs.kr
```

## Safe Patterns

```bash
# ALWAYS use --dry-run before mutating commands in production
xquare project create myproject --dry-run
xquare app create my-api --build-type go --endpoint 8080 --dry-run
xquare env set my-api KEY=value --dry-run

# Use --json for all status queries in scripts
xquare app status my-api --json
xquare addon list --json

# Use XQUARE_TOKEN env var in CI (not xquare login)
XQUARE_TOKEN=<token> XQUARE_PROJECT=myproject xquare app status my-api --json

# Check schema first if unsure about options
xquare schema
```

## Common Workflows

### Deploy a new app

```bash
# 1. Create the xquare app
xquare app create my-api --build-type go --endpoint 8080:api.dsmhs.kr \
  --owner my-org --repo my-repo --branch main

# 2. Wait ~2-3 minutes for CI infrastructure (check: xquare app status my-api)

# 3a. If code will be pushed now → just git push, CI runs automatically
git push origin main

# 3b. If code was ALREADY on GitHub before step 1 → call trigger ONCE (webhook missed it)
xquare trigger my-api

# 4. Watch progress
xquare logs my-api --build
xquare app status my-api
```

⛔ **NEVER call trigger after a git push** — webhook handles it automatically.

### Manage environment variables
```bash
xquare env get my-api --json                      # list all
xquare env set my-api DB_HOST=localhost DB_PORT=5432  # merge (non-destructive)
xquare env delete my-api OLD_KEY                  # remove specific key
xquare env pull my-api -o .env.prod               # download to file
xquare env push my-api -f .env.prod               # upload from file
```

### Database/Cache addons
```bash
xquare addon create mydb postgresql --storage 2Gi  # provision
xquare addon list --json                            # check status (ready: true/false)
xquare addon status mydb --json                     # status & connection info
xquare addon tunnel mydb                            # local port forwarding
xquare addon connect mydb                           # interactive session (mysql/psql/redis-cli)
xquare addon delete mydb --yes                      # delete (irreversible)
```

### App tunneling
```bash
xquare app tunnel my-api                  # tunnel to app's service port
xquare app tunnel my-api --port 9090      # specific port if multiple endpoints
```

### Monitoring dashboards (Grafana)
Each project has a dedicated Grafana instance. Dashboards exist for every app and addon.

```bash
xquare app dashboard my-api               # print Grafana dashboard URL for an app
xquare addon status mydb                  # includes dashboard URL in output
```

URL patterns:
```
https://{project}-observability-dashboard.dsmhs.kr/d/app-{name}    # app
https://{project}-observability-dashboard.dsmhs.kr/d/addon-{name}  # addon
```

## Addon credentials — NO password required

xquare addons have **no password**. Connect without any credentials:

| Type | DB_USER | DB_PASSWORD |
|------|---------|-------------|
| postgresql | `postgres` | (empty) |
| mysql | `root` | (empty) |
| redis | — | (no auth) |
| mongodb | — | (no auth) |

Example env vars for a postgresql addon named `db`:
```
DB_HOST=db
DB_PORT=5432
DB_USER=postgres
DB_NAME=db
# DB_PASSWORD → do NOT set, leave empty
```

## In-Cluster DNS (app ↔ app, app ↔ addon)

Apps and addons within the **same project** can communicate directly using the app/addon name as the hostname.

```
http://<app-name>:<port>          # app to app
redis://<addon-name>:6379         # app to redis
mysql://<addon-name>:3306         # app to mysql
postgresql://<addon-name>:5432    # app to postgresql
```

Examples:
```bash
# backend app calling frontend or another service
http://my-api:8080/health

# backend app connecting to its database addon named "mydb"
DB_HOST=mydb
DB_PORT=5432
```

> No namespace or full DNS suffix needed — just use the name as-is.

### Project members
```bash
xquare project members                     # list members
xquare project members add <username>      # invite by GitHub username
xquare project members remove <username> --yes  # remove member
```

### Build history & logs
```bash
xquare logs my-api --build                 # latest build logs
xquare logs my-api --build --build-id my-api-ci-abc12  # specific build
```

## Dangerous Commands (require --yes)

```bash
xquare project delete <name> --yes     # deletes ALL apps and addons
xquare app delete <name> --yes         # removes app and Vault secrets
xquare addon delete <name> --yes       # removes addon and persistent storage (irreversible!)
xquare env set --replace --yes         # overwrites ALL env vars
```

## Environment Variables (for CI)

```
XQUARE_TOKEN=<jwt>       # auth token (from xquare login, stored in ~/.xquare/config.yaml)
XQUARE_PROJECT=<name>    # default project (instead of xquare link)
XQUARE_SERVER_URL=<url>  # server URL override
CI=true                  # auto-detected: disables spinners/colors/interactive prompts
```

## MCP Integration

```bash
# Register xquare as MCP server in your AI tool (run once, then restart the IDE/tool)
xquare mcp --claude        # Claude Desktop
xquare mcp --claude-code   # Claude Code CLI  (~/.claude.json)
xquare mcp --cursor        # Cursor            (~/.cursor/mcp.json)
xquare mcp --vscode        # VS Code Copilot   (mcp.json — uses "servers" key)
xquare mcp --windsurf      # Windsurf          (~/.codeium/windsurf/mcp_config.json)
xquare mcp --zed           # Zed               (~/.config/zed/settings.json)
xquare mcp --continue      # Continue.dev      (~/.continue/config.yaml)
xquare mcp --cline         # Cline             (VS Code globalStorage)
xquare mcp --roo           # Roo Code          (VS Code globalStorage)
xquare mcp --goose         # Goose (Block)     (~/.config/goose/config.yaml)

# Multiple flags can be combined
xquare mcp --claude-code --cursor --vscode
```

### Available MCP tools

| Tool | Description |
|------|-------------|
| `list_projects` | List accessible projects |
| `get_project` | Project details (apps, addons, members) |
| `create_project` | Create project |
| `delete_project` ⚠️ | Delete project — requires `confirm="yes"` |
| `list_apps` | List apps in project |
| `get_app` | App config |
| `get_app_status` | Runtime status (running/failed, instances, version) |
| `create_app` | Create app |
| `update_app` | Update app config |
| `delete_app` ⚠️ | Delete app — requires `confirm="yes"` |
| `get_env` | Get env vars |
| `set_env` | Set env vars (merge) |
| `delete_env` | Delete env keys |
| `list_addons` | List addons |
| `create_addon` | Provision addon |
| `delete_addon` ⚠️ | Delete addon + data — requires `confirm="yes"` |
| `get_addon_status` | Addon status & connection info |
| `list_members` | List project members |
| `add_member` | Add member by GitHub username |
| `remove_member` ⚠️ | Remove member — requires `confirm="yes"` |
| `list_builds` | Build history |
| `get_build_logs` | Build logs (latest or specific build ID) |
| `get_logs` | Runtime logs |
| `trigger` | Force re-run CI/CD (normally auto on git push) |
| `get_dashboard_url` | Grafana dashboard URL for an app or addon |
| `whoami` | Current authenticated user |
| `schema` | Full CLI schema with all constraints |

### ⚠️ Destructive MCP tools — safety protocol

Tools marked ⚠️ require `confirm="yes"`. **Before setting confirm:**
1. Tell the user exactly what will be permanently deleted
2. Ask the user to explicitly confirm
3. Only set `confirm="yes"` after receiving a clear "yes"

## Output Flags (all commands)

```bash
--json              machine-readable JSON output
--jq '<expr>'       filter JSON with jq expression
--fields name,status  select specific fields
--dry-run           preview without executing
--no-input          disable interactive prompts (CI mode)
```

## Error Codes (--json mode)

```json
{"error": true, "code": "invalid_project_name", "message": "..."}
```

Codes: `auth_error` | `not_found` | `conflict` | `invalid_project_name` |
       `invalid_app_name` | `storage_too_large` | `ci_not_ready` | `timeout` | `server_error`

## Exit Codes

```
0  success
1  user error (invalid input)
3  authentication error
4  resource not found
5  conflict (name already exists)
6  server error
7  timeout
```
