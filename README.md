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
xquare upgrade                # 최신 버전으로 업데이트
```

---

## CI 환경

```sh
XQUARE_TOKEN=<token> XQUARE_PROJECT=<project> xquare app status <app> --json
```

---

## AI 연동 (MCP)

`xquare mcp` 명령어로 AI 어시스턴트가 xquare를 직접 제어할 수 있습니다.

```
"xquare에 my-api 앱 배포해줘. Go 서버고 레포는 my-org/my-api야."
"testproject의 db 애드온 연결 정보 알려줘."
"my-api 환경변수에 DATABASE_URL 추가해줘."
```

### 등록 방법

아래 명령어를 한 번 실행하면 해당 도구의 설정 파일에 자동으로 등록됩니다.

| AI 도구 | 명령어 |
|---------|--------|
| Claude Code CLI | `xquare mcp --claude-code` |
| Claude Desktop | `xquare mcp --claude` |
| Cursor | `xquare mcp --cursor` |
| VS Code (Copilot) | `xquare mcp --vscode` |
| Windsurf | `xquare mcp --windsurf` |
| Zed | `xquare mcp --zed` |
| Continue.dev | `xquare mcp --continue` |
| Cline | `xquare mcp --cline` |
| Roo Code | `xquare mcp --roo` |
| Goose | `xquare mcp --goose` |

등록 후 IDE/도구를 재시작하면 `xquare` 툴이 자동으로 나타납니다.

여러 도구에 동시 등록도 가능합니다:
```sh
xquare mcp --claude-code --cursor --vscode
```

> **참고:** AI 에이전트에게 더 정확한 문맥을 제공하려면 [`SKILL.md`](./SKILL.md)를 시스템 프롬프트에 추가하세요.

### 수동 설정 (자동 등록이 안 될 경우)

```json
{
  "mcpServers": {
    "xquare": {
      "command": "xquare",
      "args": ["mcp"]
    }
  }
}
```

VS Code는 `servers` 키를 사용합니다 (`mcpServers` 아님).
