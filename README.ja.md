<p align="center">
  <br>
  <code>██████╗ ██╗███╗   ██╗</code><br>
  <code>██╔══██╗██║████╗  ██║</code><br>
  <code>██████╔╝██║██╔██╗ ██║</code><br>
  <code>██╔══██╗██║██║╚██╗██║</code><br>
  <code>██║  ██║██║██║ ╚████║</code><br>
  <code>╚═╝  ╚═╝╚═╝╚═╝  ╚═══╝</code><br>
  <br>
  <strong>凛 — 澄み切った判断</strong><br>
  <sub>Claude Codeのためのハーネスエンジニアリングフレームワーク</sub>
</p>

<p align="center">
  <a href="#クイックスタート">クイックスタート</a> &middot;
  <a href="#仕組み">仕組み</a> &middot;
  <a href="#アーキテクチャ">アーキテクチャ</a> &middot;
  <a href="#チームモード">チームモード</a> &middot;
  <a href="#開発ワークフロー">開発ワークフロー</a> &middot;
  <a href="#コマンド">コマンド</a> &middot;
  <a href="README.md">English</a> &middot;
  <a href="README.ko.md">한국어</a>
</p>

---

RINは[Claude Code](https://github.com/anthropics/claude-code)のためのハーネスエンジニアリングフレームワークです。Markdownで定義された**エージェント**、**スキル**、**コマンド** — 構造化された制御レイヤーを追加し、汎用AIを再現可能な開発ワークフローに変えます。**永続的メモリ**（PostgreSQL + pgvector + AGEグラフ）によりハーネスはセッションを越えて学習し、**マルチモデルルーティング**（Gemini、GLM）によりコスト効率的なチーム構成が可能です。

## クイックスタート

### 1. 前提条件のインストール

| ツール | macOS | Linux |
|--------|-------|-------|
| Python 3.11+ | `brew install python@3.12` | `apt install python3 python3-venv` |
| Go 1.26+ | `brew install go` | [go.dev/dl](https://go.dev/dl/) |
| Docker | [Docker Desktop](https://www.docker.com/products/docker-desktop/) | `apt install docker.io docker-compose-plugin` |
| Ollama | `brew install ollama` | [ollama.com](https://ollama.com/) |
| Claude Code | `npm i -g @anthropic-ai/claude-code` | 同上 |

### 2. RINのインストール

```bash
git clone https://github.com/Canto87/rin.git
cd project-rin-oss
make install
```

実行順序:
1. `make check` — 前提条件の確認
2. `make setup` — Python venv作成（セッションスクリプト用）
3. `make install-db` — DockerでPostgreSQL起動 (PG17 + pgvector + AGE)
4. `make memory-go` — Goメモリサーバービルド
5. `make pull-model` — Ollama起動 + 埋め込みモデルプル (~670MB)
6. `make sync-mcp` — MCPサーバーを`~/.claude.json`に登録
7. `make install-statusline` — Claude Codeステータスラインをインストール (使用量 + メモリカウント)
8. `make install-harness-global` — エージェント/スキル/コマンドを`~/.claude/`にデプロイ (全プロジェクトで使用可能)
9. `make install-cron` — セッション収集/レビュー/整理を登録 (macOS launchd、Linuxではスキップ)
10. `make shell-setup` — `rin`をPATHに追加 (zsh/bash/fish自動検出)

### 3. 起動

```bash
source ~/.zshrc   # またはシェルを再起動
rin
```

## 仕組み

```
  rin                              # 起動
   ├─ session-picker.py            # 選択: 新規 / 再開 / コンテキスト読み込み
   ├─ rin-memory-recall            # 直近の記憶をシステムプロンプトに注入
   ├─ rin-context.md               # アイデンティティ、原則、判断の境界
   └─ claude                       # Claude Code実行
        │
        ├─ rin-memory-go (MCP)     # セマンティック検索、意思決定保存、グラフ関係
        │   ├─ PostgreSQL          #   構造化メタデータ + 全文検索
        │   ├─ pgvector            #   ベクトル埋め込み (Ollama, 1024次元)
        │   └─ AGE                 #   ナレッジグラフ (関係探索)
        │
    [セッション終了]
        │
        ├─ session-harvest         # JSONL → Markdown (launchd, 10分)
        └─ session-review          # RINが要約 → memory_store (launchd, 1時間)
```

**セッションライフサイクル:**

1. **起動** — セッションピッカーが直近のセッションを表示。再開するか、コンテキストを引き継いで新規セッションを開始。
2. **作業** — MCPツールで記憶を読み書き。意思決定やパターンが蓄積される。
3. **終了** — セッションのJSONLが自動的に構造化ノートへ収集される。
4. **レビュー** — バックグラウンドのRINインスタンスがノートを要約し、知識を抽出。
5. **次のセッション** — リコールされた記憶に過去の意思決定、未完了タスク、チームパターンが含まれる。

## アーキテクチャ

```
src/
  rin_memory_go/         # MCPサーバー (Go, PostgreSQL + pgvector + AGE)
    main.go              #   エントリポイント + MCPツール登録
    store.go             #   PostgreSQL接続 + ストレージ
    search.go            #   セマンティック + 全文ハイブリッド検索
    graph.go             #   AGEグラフ操作
    embed.go             #   Ollama埋め込み
    tools_memory.go      #   memory_* ツール (store, search, lookup, update, relate, ingest)
    tools_routing.go     #   routing_* ツール (suggest, log, stats)
    cmd_recall.go        #   recallサブコマンド (システムプロンプト注入用)
  rin_proxy/             # APIプロキシ (Go, マルチモデルルーティング)
    main.go              #   HTTPサーバー (:3456)
    openai.go            #   OpenAI互換API → 各プロバイダ変換
    passthrough.go       #   Anthropicモデルはパススルー
    streaming.go         #   SSEストリーミング対応
scripts/
  rin                    #   エントリポイント (バナー + ピッカー + claude)
  rin-team               #   チームモード (マルチプロバイダ tmux)
  rin-cc                 #   チームモード解除
  session-picker.py      #   対話型セッションセレクタ
  session-harvest.py     #   JSONL → Markdown (launchd)
  session-review.sh      #   RINによる要約 (launchd)
  memory-dream.sh        #   メモリ整理/統合 (launchd)
  sync-mcp.py            #   MCP設定 → ~/.claude.json
  sync-harness.sh        #   ハーネスを他プロジェクトまたはグローバルにデプロイ
context/
  rin-context.md         #   アイデンティティ、原則、判断の境界
launchd/                 #   macOSエージェントplist (テンプレート)
config/
  mcp-servers.json       #   MCPサーバー定義
```

### データ

| パス | 用途 |
|------|------|
| PostgreSQL `rin_memory` | ドキュメント、ベクトル、関係グラフ |
| pgvector HNSWインデックス | 1024次元セマンティック検索 |
| AGE `rin_memory` グラフ | ナレッジ関係探索 (supersedes, related, implements, contradicts) |
| `memory/sessions/` | 収集済みセッションノート (取り込み前) |

### 記憶の種類

| Kind | 説明 |
|------|------|
| `session_journal` | セッションのタイトル + 要約 |
| `arch_decision` | アーキテクチャ上の意思決定と根拠 |
| `domain_knowledge` | 外部サービスの癖、トラブルシューティング記録 |
| `team_pattern` | 協働パターン、ワークフロールール |
| `active_task` | セッション間で引き継がれる未完了タスク |
| `error_pattern` | 頻出エラーパターンと解決策 |
| `preference` | ユーザーの好み (ワークフロー、ツール、スタイル) |
| `routing_log` | モデルルーティングのパフォーマンスデータ |

## チームモード

`rin-team`はOpus（リーダー）と他プロバイダのモデル（チームメイト）を組み合わせ、マルチエージェントチームを構成します。

```bash
rin-team gemini          # チームメイト: Gemini
rin-team glm             # チームメイト: GLM
rin-team all             # opus→Gemini Pro, sonnet→GLM-5, haiku→Gemini Flash
```

```
  rin-team gemini
   │
   ├─ rin-proxy (:3456)             # APIゲートウェイ
   │
   ├─ リーダー (claude-opus-4-6)   # → proxy → Anthropic (パススルー)
   │   └─ 設計、レビュー、オーケストレーション
   │
   ├─ チームメイト (sonnet alias)   # → proxy → Gemini
   │   └─ 実装、調査、テスト
   │
   └─ チームメイト (haiku alias)    # → proxy → Gemini Flash
       └─ 軽量タスク、探索
```

**前提条件:** `make install-proxy`でrin-proxyのlaunchd登録が必要です。

## 開発ワークフロー

### 日常的な使用

```bash
rin                          # RIN起動 — 最近のセッション一覧を表示
rin --resume <session-id>    # 特定のセッションを再開
```

RINはセッション間で記憶を保持します。決定、エラーパターン、プリファレンスがメモリに保存され、次回起動時に自動的に呼び出されます。

### 組み込みコマンド

```bash
/commit          # 意味のあるメッセージで自動グループコミット
/pr              # サマリーとテストプランを含むPR作成
/code-review     # 現在の変更に対する重み付きコードレビュー
```

コマンドは`.claude/commands/`で定義され、`.claude/`のエージェントやスキルに委譲します。

### エージェント

エージェントはRINまたは相互にスポーンできる自律ワーカーです。

| エージェント | 役割 |
|------------|------|
| `code-edit` | 汎用コード修正。ファイル読み取り → 計画 → 編集 → ビルド/テスト検証。 |
| `code-review` | 読み取り専用コードレビュー。品質、セキュリティ、パターン準拠を10点満点で評価。 |
| `validate` | デュアルモード検証。(1) 設計ドキュメント vs チェックリスト一貫性。(2) 実装 vs 受入基準。 |

### スキル

スキルはエージェントとコマンドが呼び出す再利用可能なワークフローです。

| スキル | 説明 |
|-------|------|
| `auto-impl` | フェーズオーケストレーター。設計ドキュメントを読み、ビルド/テストゲート付きで実装フェーズを実行。 |
| `auto-research` | 自律実験ループ。仮説 → コード修正 → 測定 → 目標達成まで反復。 |
| `plan-feature` | 対話型設計ドキュメント生成。受入基準付きのフェーズベース計画を作成。 |
| `smart-commit` | 変更分析、レイヤー/タイプ/機能別自動グループ化、複数セマンティックコミット作成。 |
| `create-pr` | コミットからサマリー、変更分析、テストプラン付きPRを自動生成。 |
| `qa-gate` | 品質ゲート。code-review + validateを並列実行、統合スコア評価。 |
| `gc` | ガベージコレクション。デッドコード、パターンドリフト、重複、陳腐化アーティファクト検出。 |
| `troubleshoot` | 5段階診断パイプライン：症状 → 仮説 → コード検証 → 自己反駁 → 修正。 |

### ワークフロー例

```
  ユーザー: 「APIにレート制限を追加」
   │
   ├─ /plan-feature          # フェーズベースの設計ドキュメント生成
   │   └─ docs/plans/rate-limiting.md
   │
   ├─ /auto-impl             # 各フェーズを実行
   │   ├─ code-editエージェント #   変更を実装
   │   └─ qa-gate            #   フェーズごとにレビュー + 検証
   │
   ├─ /commit                # セマンティックコミットに自動グループ化
   └─ /pr                    # フルコンテキスト付きPR作成
```

### 他プロジェクトへのデプロイ

RINのハーネス（エージェント、スキル、コマンド）をプロジェクト別またはグローバルにデプロイできます:

```bash
# プロジェクト別 — target/.claude/にコピー
make sync-harness TARGET=~/workspace/other-project

# グローバル — ~/.claude/にコピー、全プロジェクトで使用可能
make sync-harness TARGET=global
```

`skill.md`ファイルのみコピーされます。プロジェクト固有の`config.yaml`は上書きされません。複数プロジェクトでRINのハーネスを使用する場合はグローバルデプロイを推奨します。

### カスタマイズ

- **`context/rin-context.md`** — 行動原則と判断境界。RINの動作方式を変更するにはこのファイルを編集します。
- **`context/rin-context-local.md`** — 環境別オーバーライド（gitignore）。共有コンテキストを変更せずにローカルルールを追加できます。`rin-context.md`の後にシステムプロンプトとして追加されます。
- **`.claude/skills/*/config.yaml`** — スキル別設定（閾値、モード）。
- **`~/.rin/memory-config.json`** — データベースDSN、Ollama URLオーバーライド。

`rin-context-local.md`の例:
```markdown
## Local Overrides
- Always respond in Japanese.
- Use Serena MCP for code navigation when available.
- Default commit messages in English.
```

## コマンド

```
make install            フルインストール (venv + MCP + モデル + Docker PG + Goビルド + launchd + PATH)
make rin                RINを起動
make test               Dockerで全パイプラインテスト (ビルド + ユニットテスト + MCPサーバー)
make test-install       Dockerでインストールパイプラインテスト (sync-mcp、ステータスライン、ハーネス、シェル設定)
```

### 個別ステップ

`make install`が全てを実行しますが、個別にも使用可能:

```
make check              前提条件の確認 (Python, Go, Docker, Ollama)
make setup              Python venv作成（セッションスクリプト用）
make install-db         DockerでPostgreSQL起動 (PG17 + pgvector + AGE)
make memory-go          Goメモリサーバーのビルド
make proxy              Goプロキシのビルド
make install-cron       セッション収集/レビュー/整理 launchd登録
make sync-mcp           MCP設定の同期
```

### 運用

```
make harvest            セッション収集 (手動)
make review             セッションレビュー (手動)
make dream              メモリ整理 (手動)
make team               チームモード: Claudeリーダー + プロバイダーチームメイト (gemini|glm|all)
make cc              チームモード終了
make sync-harness       他プロジェクトにハーネスをデプロイ (TARGET=<パス>)
make help               全ターゲットを表示
```

### オプション

```bash
# rin-proxy (チームモードの前提条件)
GEMINI_API_KEY=<key> GLM_API_KEY=<key> make install-proxy

# Ollama常時起動
make install-ollama
```

### クリーンアップ

```bash
make uninstall-db       PostgreSQLコンテナ + データ削除
make uninstall-cron     launchdエージェント削除
make uninstall-proxy    rin-proxy launchd削除
```

## ライセンス

MIT
