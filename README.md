# xquare

DSM 학생팀을 위한 PaaS CLI.

---

## 설치

**macOS**
```sh
brew install team-xquare/xquare/xquare-cli
```

**Linux**
```sh
curl -fsSL https://raw.githubusercontent.com/team-xquare/xquare-cli/main/install.sh | sh
```

**Windows** (PowerShell)
```powershell
iwr -useb https://raw.githubusercontent.com/team-xquare/xquare-cli/main/install.ps1 | iex
```

또는 [Releases](https://github.com/team-xquare/xquare-cli/releases)에서 직접 다운로드.

---

## 시작하기

```sh
xquare login                  # GitHub 인증
xquare link <project>         # 현재 디렉토리에 프로젝트 연결
xquare app list               # 앱 목록
xquare trigger <app>          # CI/CD 수동 트리거
```

---

## CI 환경

```sh
XQUARE_TOKEN=<token> XQUARE_PROJECT=<project> xquare app status <app> --json
```

---

## AI 에이전트 연동

```sh
xquare mcp --claude    # Claude Desktop
xquare mcp --cursor    # Cursor
xquare mcp --vscode    # VS Code
```

MCP 등록 후 IDE 재시작. [`SKILL.md`](./SKILL.md)를 AI 컨텍스트에 추가하면 더 정확하게 동작합니다.
