# CI Test TODO (Agent Ops)

AIエージェント運用向けの安全機能を、段階的にCI自動検証へ落とし込むためのTODO。

## Phase 1: Fast Unit (PR必須)

- [ ] `internal/cmd/auth.go`
  - [ ] `auth preflight` が `ready` と `checks[]` を返す（credentials/token/policy）
  - [ ] `auth approval-token` の `ttl` パース失敗時に `invalid_ttl`
  - [ ] `auth emergency-revoke` で policy に blocked account が追記される
- [ ] `internal/cmd/policy.go`
  - [ ] `allowed_actions` 許可/拒否の分岐
  - [ ] `blocked_accounts` 拒否の分岐
  - [ ] `require_approval_actions` の上書き挙動
- [ ] `internal/cmd/approval.go`
  - [ ] ワンタイムトークン消費（2回目失敗）
  - [ ] 期限切れトークン拒否
  - [ ] account/action 不一致拒否
- [ ] `internal/cmd/docs.go`
  - [ ] `--allowed-output-dir` 外への export 拒否（`output_not_allowed`）
  - [ ] `--replace` 時 `--confirm-replace` 必須
  - [ ] `docs find-replace` 時 `--confirm-find-replace` 必須
- [ ] `internal/cmd/calendar.go`
  - [ ] `calendar delete` 時 `--confirm-delete` 必須
- [ ] `internal/cmd/ratelimit.go`
  - [ ] window内上限超過で `rate_limited`
- [ ] `internal/googleapi/client.go`
  - [ ] read/write コンストラクタが意図した scope で TokenSource を生成する

## Phase 2: CLI Integration (PR推奨)

- [ ] 一時 `HOME`/`XDG_CONFIG_HOME` を使った black-box テストを追加
- [ ] `gog-lite auth preflight --account ...` の JSON スキーマ検証
- [ ] policy で拒否された write コマンドが `policy_denied` を返す
  - [ ] `gmail send`
  - [ ] `calendar create/update/delete`
  - [ ] `docs create/write/find-replace/export`
- [ ] approval-token 必須アクションで token 未指定/再利用時の `approval_required` 検証

## Phase 3: Security Regression (nightly)

- [ ] 監査ログのチェーン改ざん検知テスト（`prev_hash` / `hash`）
- [ ] `GOG_LITE_KEYRING_BACKEND=file` かつ `GOG_LITE_KEYRING_PASSWORD` 未設定時の失敗
- [ ] 大量入力 (`--body-stdin`, `--content-stdin`) の上限制御検証
- [ ] 危険操作の「dry-runではtoken不要・実行時はtoken必須」挙動確認

## Workflow TODO

- [ ] `.github/workflows/ci.yml` に以下ジョブを追加
  - [ ] `unit-fast` (`go test ./... -short`)
  - [ ] `integration-cli` (temp HOMEでCLI実行)
  - [ ] `security-nightly` (schedule)
- [ ] 失敗時に JSON エラー本文を artifacts 保存
- [ ] `README.md` に CI セキュリティテスト方針を短く追記

