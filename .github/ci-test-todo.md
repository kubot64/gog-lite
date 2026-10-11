# CI Test TODO (Agent Ops)

AIエージェント運用向けの安全機能を CI で検証するための残作業。チェック済みは実装済み。

## Phase 1: Fast Unit (PR必須)

- [x] `internal/cmd/auth.go`
  - [x] `auth preflight` が `ready` と `checks[]` を返す（credentials/token/policy）
  - [x] `auth approval-token` の `ttl` パース失敗時に `invalid_ttl`
  - [x] `auth emergency-revoke` で policy に blocked account が追記される
- [x] `internal/cmd/policy.go`
  - [x] `allowed_actions` 許可/拒否の分岐
  - [x] `blocked_accounts` 拒否の分岐
  - [x] `require_approval_actions` は既定の承認必須との和集合（免除は `exempt_approval_actions`）
- [x] `internal/cmd/approval.go`
  - [x] ワンタイムトークン消費（2回目失敗）
  - [x] 期限切れトークン拒否
  - [x] account/action 不一致拒否
- [x] `internal/cmd/docs.go`
  - [x] `--allowed-output-dir` 外への export 拒否（`output_not_allowed`）
  - [x] `--replace` 時 `--confirm-replace` 必須
  - [x] `docs find-replace` 時 `--confirm-find-replace` 必須
- [x] `internal/cmd/calendar.go`
  - [x] `calendar delete` 時 `--confirm-delete` 必須
- [x] `internal/cmd/ratelimit.go`
  - [x] window内上限超過で `rate_limited`
- [ ] `internal/googleapi/client.go`
  - [ ] read/write コンストラクタが意図した scope で TokenSource を生成する
  - ログイン時スコープに実行時スコープが含まれることは `TestRuntimeScopesAreGrantedAtLogin` で見ている。コンストラクタが TokenSource に渡す scope 自体は未検証

## Phase 2: CLI Integration (PR推奨)

- [x] 一時 `HOME`/`XDG_CONFIG_HOME` を使った black-box テストを追加
- [x] `gog-lite auth preflight --account ...` の JSON スキーマ検証
- [x] policy で拒否された write コマンドが `policy_denied` を返す
  - [x] `gmail send`
  - [x] `calendar create/update/delete`
  - [x] `docs create/write/find-replace/export`
  - [x] `sheets update/append`
  - [x] `slides write`
- [x] approval-token 必須アクションで token 未指定/再利用時の `approval_required` 検証
  - 再利用はトークンファイルの `used` を見てから、2 回目のコマンドを 1 回だけ実行する

## Phase 3: Security Regression (nightly)

- [x] 監査ログのチェーン改ざん検知テスト（`prev_hash` / `hash`）
- [x] `GOG_LITE_KEYRING_BACKEND=file` かつ `GOG_LITE_KEYRING_PASSWORD` 未設定時の失敗
- [x] 大量入力 (`--body-stdin`, `--content-stdin`) の上限制御検証（`readStdinWithLimit`）
- [x] 危険操作の「dry-runではtoken不要・実行時はtoken必須」挙動確認

## Workflow TODO

- [x] `.github/workflows/ci.yml` に以下ジョブを追加
  - [x] `unit-fast` (`go test ./...` と `go vet`)
  - [x] `staticcheck`（PR / push。`U1000` と `S1001` は除外）
  - [x] `govulncheck`（PR / push と毎日 02:00 UTC）
  - [x] `integration-cli` (temp HOMEでCLI実行)
  - [x] `security-nightly` (schedule)
- [ ] 失敗時に JSON エラー本文を artifacts 保存
  - `unit-fast` は `/tmp/gog-lite-test-*.log` を上げるが、テストはそのパスへ書いていない。失敗時の本文はジョブログの stderr にある
- [x] `README.md` に CI セキュリティテスト方針を短く追記
