# xquare-cli

Go CLI 도구. 부모 디렉토리 CLAUDE.md의 규칙을 모두 따른다.

## 이 디렉토리에서 작업 시 추가 규칙

### 새 커맨드 → `/feature-dev` 먼저 실행

### 출력 규칙 (매우 중요)
```
stdout = 데이터 (--json 시 JSON, human 시 테이블)
stderr = 진행상황, 에러 메시지
```
- `--json` 플래그: 모든 커맨드에 지원, 순수 JSON만 stdout
- TTY 감지: `isatty.IsTerminal(os.Stdout.Fd())`
- `CI=true` 또는 non-TTY → 색상/스피너/인터랙션 비활성화

### 패키지별 책임
```
cmd/            Cobra 커맨드 정의 (얇게, 로직은 internal/)
internal/api/   서버 HTTP 클라이언트
internal/config/ ~/.xquare/config.yaml + .xquare/config
internal/output/ human/json/ndjson 출력 (TTY 감지)
internal/tunnel/ wstunnel 클라이언트 (DB 터널)
internal/mcp/   MCP 서버 도구 정의
```

### 필수 플래그 (모든 커맨드)
- `--json`: JSON 출력
- `--jq`: gojq 내장 필터
- `--fields`: 응답 필드 선택
- mutating 커맨드: `--dry-run`, `--yes`

### 에러 메시지 형식
```
Error: {무엇이 잘못됐는지}

{왜 잘못됐는지 (알 수 있으면)}

  {다음 단계 명령어}    {설명}
  {대안 명령어}         {설명}
```

### Exit Code
```
0 = 성공
1 = 사용자 오류
2 = 사용법 오류
3 = 인증 오류
4 = 리소스 없음
5 = 충돌
6 = 서버 오류
7 = 타임아웃
```
