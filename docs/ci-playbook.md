# CI Playbook

CI 失敗時の調査・対処手順。

## 基本手順

1. 失敗ジョブを特定する（Unit / CLI Integration / Security nightly）。
2. failed step のログを取得して、最初の失敗行を確認する。
3. 実装起因かフレークかを切り分ける。
4. フレーク疑い時は rerun して再現性を確認する。

## よくある失敗

### `JSONDecodeError`（stderr 空）

- 症状: 失敗系テストで `json.load(...)` が空ファイルを読んで落ちる。
- 対処:
  - `err_json=$(mktemp)` を使う。
  - `cat "$err_json"` 前に `[ -s "$err_json" ]` で非空チェック。
  - 共有 `/tmp/err.json` を使わない。

### auth/keyring 系失敗

- 症状: `keyring_error`、`auth_required`、token 取得失敗。
- 対処:
  - CI で必要な環境変数を確認（`GOG_LITE_CLIENT_ID`, `GOG_LITE_CLIENT_SECRET`）。
  - file backend 利用時は `GOG_LITE_KEYRING_BACKEND=file` と `GOG_LITE_KEYRING_PASSWORD` を設定。

### 時刻・入力境界

- 症状: RFC3339 判定、stdin 上限テストが不安定。
- 対処:
  - テストで時刻依存を固定化。
  - 境界値（ちょうど上限 / 上限+1）を分離検証。

## 実行コマンド例

```bash
# 変更範囲のユニットテスト
go test ./internal/cmd -run 'TestName'

# 全体
GOCACHE=/tmp/gocache go test ./...
```

## マージ判定

- 必須チェックが green であること。
- フレーク再実行で通った場合でも、再発防止PRを別途作成する。
