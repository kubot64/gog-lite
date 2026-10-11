# CI Playbook

CI 失敗時の調査・対処手順。

## 基本手順

1. 失敗ジョブを特定する（Unit / CLI Integration / govulncheck / Security nightly）。
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

### govulncheck

- 症状: `govulncheck` が終了コード 3 で、呼び出し経路上の脆弱性を報告する。
- 対処:
  - 標準ライブラリなら `go` 行（と必要なら `toolchain`）を修正済みパッチへ上げる。
  - モジュールなら該当依存を修正版へ上げて `go mod tidy` する。
  - 呼び出し経路なしの検出は既定の symbol スキャンでは失敗にしない。`-show verbose` で残件を確認する。

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

## schedule が無効になったとき

公開リポジトリでは、60 日コミットなどの操作が無いと GitHub が `schedule` を無効にする。対象は `ci.yml`（毎日 02:00 UTC）と `codeql.yml`（毎週月曜 03:00 UTC）。push / pull_request の CI は対象外。無効になったあとは、その cron 自身では復活しない。

両ワークフローの keepalive ジョブは、`schedule` と `workflow_dispatch` の実行のたびに次を呼ぶ。権限はそのジョブだけの `actions: write`。空コミットは使わない。

- `PUT /repos/{owner}/{repo}/actions/workflows/ci.yml/enable`
- `PUT /repos/{owner}/{repo}/actions/workflows/codeql.yml/enable`

一度止まるとこのジョブも走らない。Actions 画面で戻す。

1. リポジトリの **Actions** タブを開く。
2. 左の一覧で **CI** または **CodeQL** を選ぶ。
3. 無効化の案内と **Enable workflow** が出ている場合は、そのボタンを押す。
4. 同じ画面の **Run workflow** でブランチ `main` を選び、手動実行する。次の cron を待たずに keepalive が走る。CI の手動実行は通常の CI ジョブと nightly と keepalive をまとめて走らせ、CodeQL の手動実行は解析と keepalive を走らせる。
5. CI と CodeQL の両方を確認する。片方が無効のままだと、そちらの cron は戻らない。

## Actions の許可リスト

ワークフロー内のアクションはコミット SHA にピンする。不変性は YAML のピンが担う。リポジトリ設定の許可リストはリポジトリ単位のワイルドカードにし、Dependabot が SHA を更新しても設定の追記が要らないようにする。

設定変更はリポジトリの Actions 管理権限が要る。コードからは変更しない。

1. **Settings**（設定）→ 左の **Actions** → **General**（一般）を開く。
2. **Actions permissions**（アクションの権限）で **Allow specific actions and reusable workflows**（指定したアクションと再利用可能なワークフローを許可。画面によっては Allow specified actions）を選ぶ。
3. テキストボックスのフル SHA 行を、次のパターンに置き換える。1 行に 1 パターン。

```
actions/checkout@*
actions/setup-go@*
actions/upload-artifact@*
rhysd/actionlint@*
goreleaser/goreleaser-action@*
peter-evans/create-pull-request@*
github/codeql-action/*@*
```

4. **Save**（保存）する。

`actions/upload-artifact` は `ci.yml` が使っている。パターンから漏れると、そのワークフローは startup_failure になる。フル SHA 行を残すと、Dependabot の SHA 更新のたびに許可リストの追記がまた必要になる。SHA 以外の既存パターンがある場合は消さず、SHA 行だけを上のワイルドカードに置き換える。

YAML の SHA ピンは外さない。**Require actions to be pinned to a full-length commit SHA** が有効なら、そのままでよい。

`.github/dependabot.yml` の `github-actions` は `groups` でまとめ、アクション更新を 1 本の PR にする。`check-action-refs.sh` は SHA が解決できることに加え、`# v6` のようなコメントがそのコミットのタグ（`v6` または `v6.0.2`）と一致するかを確認する。

## マージ判定

- 必須チェックが green であること。
- フレーク再実行で通った場合でも、再発防止PRを別途作成する。
