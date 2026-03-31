<p align="center">
  <br>
  <code>██████╗ ██╗███╗   ██╗</code><br>
  <code>██╔══██╗██║████╗  ██║</code><br>
  <code>██████╔╝██║██╔██╗ ██║</code><br>
  <code>██╔══██╗██║██║╚██╗██║</code><br>
  <code>██║  ██║██║██║ ╚████║</code><br>
  <code>╚═╝  ╚═╝╚═╝╚═╝  ╚═══╝</code><br>
  <br>
  <strong>凛 — 맑고 또렷한</strong><br>
  <sub>Claude Code를 위한 하네스 엔지니어링 프레임워크</sub>
</p>

<p align="center">
  <a href="#빠른-시작">빠른 시작</a> &middot;
  <a href="#동작-원리">동작 원리</a> &middot;
  <a href="#구조">구조</a> &middot;
  <a href="#팀-모드">팀 모드</a> &middot;
  <a href="#개발-워크플로우">개발 워크플로우</a> &middot;
  <a href="#커맨드">커맨드</a> &middot;
  <a href="README.md">English</a>
</p>

---

RIN은 [Claude Code](https://github.com/anthropics/claude-code)를 위한 하네스 엔지니어링 프레임워크입니다. 마크다운으로 정의된 **에이전트**, **스킬**, **커맨드** — 구조화된 제어 레이어를 추가하여 범용 AI를 재현 가능한 개발 워크플로우로 만듭니다. **영속적 기억** (PostgreSQL + pgvector + AGE 그래프)은 하네스가 세션을 넘어 학습하게 하고, **멀티 모델 라우팅** (Gemini, GLM)은 비용 효율적 팀 구성을 가능하게 합니다.

## 빠른 시작

### 1. 전제조건 설치

| 도구 | macOS | Linux |
|------|-------|-------|
| Python 3.11+ | `brew install python@3.12` | `apt install python3 python3-venv` |
| Go 1.26+ | `brew install go` | [go.dev/dl](https://go.dev/dl/) |
| Docker | [Docker Desktop](https://www.docker.com/products/docker-desktop/) | `apt install docker.io docker-compose-plugin` |
| Ollama | `brew install ollama` | [ollama.com](https://ollama.com/) |
| Claude Code | `npm i -g @anthropic-ai/claude-code` | 동일 |

### 2. RIN 설치

```bash
git clone https://github.com/Canto87/rin.git
cd project-rin-oss
make install
```

실행 순서:
1. `make check` — 전제조건 확인
2. `make setup` — Python venv 생성 (세션 스크립트용)
3. `make install-db` — Docker로 PostgreSQL 시작 (PG17 + pgvector + AGE)
4. `make memory-go` — Go 메모리 서버 빌드
5. `make pull-model` — Ollama 시작 + 임베딩 모델 풀 (~670MB)
6. `make sync-mcp` — MCP 서버를 `~/.claude.json`에 등록
7. `make install-statusline` — Claude Code 상태표시줄 설치 (사용량 + 메모리 카운트)
8. `make install-harness-global` — 에이전트/스킬/커맨드를 `~/.claude/`에 배포 (모든 프로젝트에서 사용 가능)
9. `make install-cron` — 세션 수확/리뷰/정리 등록 (macOS launchd, Linux는 스킵)
10. `make shell-setup` — `rin`을 PATH에 추가 (zsh/bash/fish 자동 감지)

### 3. 실행

```bash
source ~/.zshrc   # 또는 쉘 재시작
rin
```

## 동작 원리

```
  rin                              # 시작
   ├─ session-picker.py            # 선택: 새 세션 / 이어가기 / 맥락 로드
   ├─ rin-memory-recall            # 최근 기억을 시스템 프롬프트에 주입
   ├─ rin-context.md               # 정체성, 원칙, 판단 경계
   └─ claude                       # Claude Code 실행
        │
        ├─ rin-memory-go (MCP)     # 시맨틱 검색, 결정 저장, 그래프 관계
        │   ├─ PostgreSQL          #   구조화 메타데이터 + 풀텍스트
        │   ├─ pgvector            #   벡터 임베딩 (Ollama, 1024차원)
        │   └─ AGE                 #   지식 그래프 (관계 탐색)
        │
    [세션 종료]
        │
        ├─ session-harvest         # JSONL → 마크다운 (launchd, 10분)
        └─ session-review          # 린이 요약 → memory_store (launchd, 1시간)
```

**세션 라이프사이클:**

1. **시작** — 세션 선택기가 최근 세션을 표시. 이어가기 또는 맥락만 가져와서 새 세션 시작.
2. **작업** — MCP 도구로 기억을 읽고 씀. 결정과 패턴이 축적됨.
3. **종료** — 세션 JSONL이 자동으로 구조화 노트로 수확됨.
4. **리뷰** — 백그라운드 린 인스턴스가 노트를 요약하고 지식을 추출.
5. **다음 세션** — 리콜된 기억에 과거 결정, 미완료 작업, 팀 패턴이 포함됨.

## 구조

```
src/
  rin_memory_go/         # MCP 서버 (Go, PostgreSQL + pgvector + AGE)
    main.go              #   엔트리포인트 + MCP 도구 등록
    store.go             #   PostgreSQL 연결 + 스토리지
    search.go            #   시맨틱 + 풀텍스트 하이브리드 검색
    graph.go             #   AGE 그래프 연산
    embed.go             #   Ollama 임베딩
    tools_memory.go      #   memory_* 도구 (store, search, lookup, update, relate, ingest)
    tools_routing.go     #   routing_* 도구 (suggest, log, stats)
    cmd_recall.go        #   recall 서브커맨드 (시스템 프롬프트 주입용)
  rin_proxy/             # API 프록시 (Go, 멀티 모델 라우팅)
    main.go              #   HTTP 서버 (:3456)
    openai.go            #   OpenAI 호환 API → 각 프로바이더 변환
    passthrough.go       #   Anthropic 모델은 패스스루
    streaming.go         #   SSE 스트리밍 지원
scripts/
  rin                    #   엔트리포인트 (배너 + 선택기 + claude)
  rin-team               #   팀 모드 (멀티 프로바이더 tmux)
  rin-cc                 #   팀 모드 해제
  session-picker.py      #   대화형 세션 선택기
  session-harvest.py     #   JSONL → 마크다운 (launchd)
  session-review.sh      #   린이 요약 (launchd)
  memory-dream.sh        #   메모리 정리/통합 (launchd)
  sync-mcp.py            #   MCP 설정 → ~/.claude.json
  sync-harness.sh        #   하네스를 다른 프로젝트 또는 글로벌 배포
context/
  rin-context.md         #   정체성, 원칙, 판단 경계
launchd/                 #   macOS 에이전트 plist (템플릿)
config/
  mcp-servers.json       #   MCP 서버 정의
```

### 데이터

| 경로 | 용도 |
|------|------|
| PostgreSQL `rin_memory` | 문서, 벡터, 관계 그래프 |
| pgvector HNSW 인덱스 | 1024차원 시맨틱 검색 |
| AGE `rin_memory` 그래프 | 지식 관계 탐색 (supersedes, related, implements, contradicts) |
| `memory/sessions/` | 수확된 세션 노트 (취입 전) |

### 기억 종류

| Kind | 설명 |
|------|------|
| `session_journal` | 세션 제목 + 요약 |
| `arch_decision` | 아키텍처 결정과 근거 |
| `domain_knowledge` | 외부 서비스 quirk, 트러블슈팅 기록 |
| `team_pattern` | 협업 패턴, 워크플로우 규칙 |
| `active_task` | 세션 간 이월되는 미완료 작업 |
| `error_pattern` | 반복 에러 패턴과 해결법 |
| `preference` | 사용자 선호 (워크플로우, 도구, 스타일) |
| `routing_log` | 모델 라우팅 성능 데이터 |

## 팀 모드

`rin-team`은 Opus(리더) + 다른 프로바이더 모델(팀메이트)을 조합하여 멀티 에이전트 팀을 구성합니다.

```bash
rin-team gemini          # 팀메이트: Gemini
rin-team glm             # 팀메이트: GLM
rin-team all             # opus→Gemini Pro, sonnet→GLM-5, haiku→Gemini Flash
```

```
  rin-team gemini
   │
   ├─ rin-proxy (:3456)             # API 게이트웨이
   │
   ├─ 리더 (claude-opus-4-6)       # → proxy → Anthropic (패스스루)
   │   └─ 설계, 리뷰, 오케스트레이션
   │
   ├─ 팀메이트 (sonnet alias)      # → proxy → Gemini
   │   └─ 구현, 조사, 테스트
   │
   └─ 팀메이트 (haiku alias)       # → proxy → Gemini Flash
       └─ 빠른 작업, 탐색
```

**전제조건:** `make install-proxy`로 rin-proxy launchd 등록 필요.

## 개발 워크플로우

### 일상 사용

```bash
rin                          # 린 실행 — 최근 세션 목록 표시
rin --resume <session-id>    # 특정 세션 이어하기
```

린은 세션 간 기억을 유지합니다. 결정, 에러 패턴, 선호도가 메모리에 저장되고 다음 실행 시 자동으로 불러옵니다.

### 내장 커맨드

```bash
/commit          # 의미 있는 메시지로 자동 그룹 커밋
/pr              # 요약과 테스트 계획이 포함된 PR 생성
/code-review     # 현재 변경사항에 대한 가중치 기반 코드 리뷰
```

커맨드는 `.claude/commands/`에 정의되어 있으며, `.claude/`의 에이전트나 스킬에 위임합니다.

### 에이전트

에이전트는 린이 또는 서로가 스폰할 수 있는 자율 워커입니다.

| 에이전트 | 역할 |
|---------|------|
| `code-edit` | 범용 코드 수정. 파일 읽기 → 계획 → 편집 → 빌드/테스트 검증. |
| `code-review` | 읽기 전용 코드 리뷰. 품질, 보안, 패턴 준수를 10점 만점으로 평가. |
| `validate` | 이중 모드 검증. (1) 설계 문서 vs 체크리스트 일관성. (2) 구현 vs 수용 기준. |

### 스킬

스킬은 에이전트와 커맨드가 호출하는 재사용 가능한 워크플로우입니다.

| 스킬 | 설명 |
|------|------|
| `auto-impl` | 페이즈 오케스트레이터. 설계 문서를 읽고 빌드/테스트 게이트와 함께 구현 페이즈 실행. |
| `auto-research` | 자율 실험 루프. 가설 → 코드 수정 → 측정 → 목표 달성까지 반복. |
| `plan-feature` | 대화형 설계 문서 생성기. 수용 기준이 포함된 페이즈 기반 계획 작성. |
| `smart-commit` | 변경사항 분석, 레이어/타입/기능별 자동 그룹화, 복수 시맨틱 커밋 생성. |
| `create-pr` | 커밋에서 요약, 변경 분석, 테스트 계획이 포함된 PR 자동 생성. |
| `qa-gate` | 품질 게이트. code-review + validate를 병렬 실행, 결합 점수 평가. |
| `gc` | 가비지 컬렉션. 데드 코드, 패턴 드리프트, 중복, 스테일 아티팩트 탐지. |
| `troubleshoot` | 5단계 진단 파이프라인: 증상 → 가설 → 코드 검증 → 자기 반박 → 수정. |

### 워크플로우 예시

```
  사용자: "API에 레이트 리미팅 추가"
   │
   ├─ /plan-feature          # 페이즈 기반 설계 문서 생성
   │   └─ docs/plans/rate-limiting.md
   │
   ├─ /auto-impl             # 각 페이즈 실행
   │   ├─ code-edit 에이전트  #   변경사항 구현
   │   └─ qa-gate            #   페이즈별 리뷰 + 검증
   │
   ├─ /commit                # 시맨틱 커밋으로 자동 그룹화
   └─ /pr                    # 전체 맥락이 포함된 PR 생성
```

### 다른 프로젝트에 배포

린의 하네스(에이전트, 스킬, 커맨드)를 프로젝트별 또는 글로벌로 배포할 수 있습니다:

```bash
# 프로젝트별 — target/.claude/에 복사
make sync-harness TARGET=~/workspace/other-project

# 글로벌 — ~/.claude/에 복사, 모든 프로젝트에서 사용 가능
make sync-harness TARGET=global
```

`skill.md` 파일만 복사됩니다. 프로젝트별 `config.yaml`은 덮어쓰지 않습니다. 여러 프로젝트에서 린의 하네스를 사용한다면 글로벌 배포를 권장합니다.

### 커스터마이즈

- **`context/rin-context.md`** — 행동 원칙과 판단 경계. 린의 동작 방식을 바꾸려면 이 파일을 수정합니다.
- **`context/rin-context-local.md`** — 환경별 오버라이드 (gitignore). 공유 컨텍스트를 수정하지 않고 로컬 규칙을 추가할 수 있습니다. `rin-context.md` 뒤에 시스템 프롬프트로 추가됩니다.
- **`.claude/skills/*/config.yaml`** — 스킬별 설정 (임계값, 모드).
- **`~/.rin/memory-config.json`** — 데이터베이스 DSN, Ollama URL 오버라이드.

`rin-context-local.md` 예시:
```markdown
## Local Overrides
- Always respond in Korean.
- Use Serena MCP for code navigation when available.
- Default commit messages in English.
```

## 커맨드

```
make install            전체 설치 (venv + MCP + 모델 + Docker PG + Go 빌드 + launchd + PATH)
make rin                린 실행
make test               Docker에서 전체 파이프라인 테스트 (빌드 + 단위 테스트 + MCP 서버)
make test-install       Docker에서 인스톨 파이프라인 테스트 (sync-mcp, 상태표시줄, 하네스, 쉘 설정)
```

### 개별 단계

`make install`이 전부 실행하지만, 개별로도 사용 가능:

```
make check              전제조건 확인 (Python, Go, Docker, Ollama)
make setup              Python venv 생성 (세션 스크립트용)
make install-db         Docker로 PostgreSQL 시작 (PG17 + pgvector + AGE)
make memory-go          Go 메모리 서버 빌드
make proxy              Go 프록시 빌드
make install-cron       세션 수확/리뷰/정리 launchd 등록
make sync-mcp           MCP 설정 동기화
```

### 운영

```
make harvest            세션 수확 (수동)
make review             세션 리뷰 (수동)
make dream              메모리 정리 (수동)
make team               팀 모드: Claude 리더 + 프로바이더 팀메이트 (gemini|glm|all)
make cc              팀 모드 종료
make sync-harness       다른 프로젝트에 하네스 배포 (TARGET=<경로>)
make help               전체 타겟 표시
```

### 선택적 설치

```bash
# rin-proxy (팀 모드 전제조건)
GEMINI_API_KEY=<key> GLM_API_KEY=<key> make install-proxy

# Ollama 상시 실행
make install-ollama
```

### 정리

```bash
make uninstall-db       PostgreSQL 컨테이너 + 데이터 삭제
make uninstall-cron     launchd 에이전트 제거
make uninstall-proxy    rin-proxy launchd 제거
```

## 라이선스

MIT
