# gog-lite — Codex Agent Guide

このファイルは **Codex 向けの最小運用ルール** です。詳細仕様は `docs/` 配下を参照。

## 優先順位

1. `AGENTS.md`（本ファイル）
2. `docs/*.md`（詳細ルール・手順）
3. `README.md`（利用者向け説明）

矛盾がある場合は上から優先して判断する。

## 必須ルール

- stdout は常に JSON。色・表・TSV を出力しない。
- エラーは stderr に `{"error":"...","code":"..."}` を出力する。
- 終了コードは `0=成功 / 1=エラー / 2=認証エラー / 3=未発見 / 4=権限なし` を守る。
- アカウント対象コマンドでは `--account` は必須（グローバルではない。`auth list` などは除く）。
- 書き込み系の検証では `--dry-run` を優先して使う。
- 破壊的操作（削除・置換・find-replace）には confirm フラグ・approval-token 要件を維持する。

## コーディング時の注意

- 既存スタイルに合わせ、最小差分で修正する。
- 無関係なリファクタやファイル移動を同PRに混ぜない。
- セキュリティ機能（policy / approval-token / audit-log / ratelimit）の後方互換を壊さない。
- 秘密情報（credentials、token、パスワード）をコード・ログ・PR本文に含めない。
- CI失敗系テストでは共有 `/tmp` 固定ファイルを避け、`mktemp` と非空チェックを使う。

## テスト・レビュー方針

- 変更箇所に対応するテストを最小で実行し、必要なら `go test ./...` まで拡張する。
- レビューはバグ・回帰・テスト不足を優先し、見た目変更は後回しにする。
- PRには「目的 / 変更点 / テスト結果 / 影響範囲」を明記する。

## 詳細ドキュメント

- 仕様リファレンス: [`docs/agents.md`](docs/agents.md)
- コーディング規約: [`docs/coding-rules.md`](docs/coding-rules.md)
- レビュー観点: [`docs/review-checklist.md`](docs/review-checklist.md)
- CI運用手順: [`docs/ci-playbook.md`](docs/ci-playbook.md)
- アーキテクチャ判断（ADR）: [`docs/adr/README.md`](docs/adr/README.md)
- 初期セットアップ: [`docs/getting-started.md`](docs/getting-started.md)

<!-- BEGIN BEADS INTEGRATION -->
## Issue Tracking with bd (beads)

**IMPORTANT**: This project uses **bd (beads)** for ALL issue tracking. Do NOT use markdown TODOs, task lists, or other tracking methods.

### Why bd?

- Dependency-aware: Track blockers and relationships between issues
- Git-friendly: Dolt-powered version control with native sync
- Agent-optimized: JSON output, ready work detection, discovered-from links
- Prevents duplicate tracking systems and confusion

### Quick Start

**Check for ready work:**

```bash
bd ready --json
```

**Create new issues:**

```bash
bd create "Issue title" --description="Detailed context" -t bug|feature|task -p 0-4 --json
bd create "Issue title" --description="What this issue is about" -p 1 --deps discovered-from:bd-123 --json
```

**Claim and update:**

```bash
bd update <id> --claim --json
bd update bd-42 --priority 1 --json
```

**Complete work:**

```bash
bd close bd-42 --reason "Completed" --json
```

### Issue Types

- `bug` - Something broken
- `feature` - New functionality
- `task` - Work item (tests, docs, refactoring)
- `epic` - Large feature with subtasks
- `chore` - Maintenance (dependencies, tooling)

### Priorities

- `0` - Critical (security, data loss, broken builds)
- `1` - High (major features, important bugs)
- `2` - Medium (default, nice-to-have)
- `3` - Low (polish, optimization)
- `4` - Backlog (future ideas)

### Workflow for AI Agents

1. **Check ready work**: `bd ready` shows unblocked issues
2. **Claim your task atomically**: `bd update <id> --claim`
3. **Work on it**: Implement, test, document
4. **Discover new work?** Create linked issue:
   - `bd create "Found bug" --description="Details about what was found" -p 1 --deps discovered-from:<parent-id>`
5. **Complete**: `bd close <id> --reason "Done"`

### Auto-Sync

bd automatically syncs via Dolt:

- Each write auto-commits to Dolt history
- Use `bd dolt push`/`bd dolt pull` for remote sync
- No manual export/import needed!

### Important Rules

- ✅ Use bd for ALL task tracking
- ✅ Always use `--json` flag for programmatic use
- ✅ Link discovered work with `discovered-from` dependencies
- ✅ Check `bd ready` before asking "what should I work on?"
- ❌ Do NOT create markdown TODO lists
- ❌ Do NOT use external issue trackers
- ❌ Do NOT duplicate tracking systems

For more details, see README.md and docs/QUICKSTART.md.

<!-- END BEADS INTEGRATION -->

## Landing the Plane (Session Completion)

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   bd sync
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
