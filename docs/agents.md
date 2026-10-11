# gog-lite — AI Agent Reference

AIエージェントが gog-lite を使う際に必要な情報をまとめたリファレンス。

---

## 基本原則

| 原則 | 内容 |
|------|------|
| stdout | 常に JSON（色・表・TSVなし）。`--help` / `--version` も JSON |
| stderr | エラーのみ `{"error": "...", "code": "..."}`（引数エラーを含む） |
| 終了コード | 0=成功 / 1=エラー / 2=認証エラー / 3=未発見 / 4=権限なし |
| 認証 | ブラウザ不要。2ステップで URL → コード交換 |
| `--account` | アカウント対象コマンドで必須（`auth list` は不要） |
| `--dry-run` | グローバルフラグ。API呼び出しなしで実行内容を確認 |
| `--audit-log` | グローバルフラグ。書き込み系操作の監査ログ(JSONL) |
| `--allowed-output-dir` | グローバルフラグ。ファイル出力先ディレクトリ制限 |

---

## Stable Contract

AI や自動化が安定して依存してよい契約は次の通り。

- `stdout` は成功時 JSON のみを返す
- `stderr` は失敗時 JSON エラーのみを返す
- 終了コードは `0/1/2/3/4` に固定する
- アカウント対象コマンドでは `--account` を必須とする
- 書き込み系操作は `--dry-run` を標準の事前確認手段とする
- 破壊的操作は `confirm` と必要に応じて `approval-token` で保護する
- `sheets update` と `sheets append` はセル値を既定で `RAW` として書き込む。数式や日付として解釈するのは `--value-input USER_ENTERED` を付けたときだけ
- `sheets update` は `--confirm-write` と、既定では `--approval-token` を要求する。`--dry-run` ではどちらも不要
- `docs write --replace` は `--confirm-replace` と、既定では `--approval-token` を要求する。`--dry-run` ではどちらも不要で、API は呼ばない
- `--audit-log` と `--allowed-output-dir` は安全制御として扱う
- 監査ログの新規行は `hash_alg` が `hmac-sha256`。`hash_alg` の無い既存行は SHA-256 のまま読む
- 監査ログの追記には keyring が必要。失敗時の code は `audit_error`（終了コード 1）
- ファイルバックエンドでは `GOG_LITE_KEYRING_PASSWORD` が必須。`GOG_LITE_KEYRING_BACKEND=file` のときと、Linux でセッションバスが無いときの自動選択のどちらも同じ。未設定・空・空白のみは `keyring_error`（終了コード 1）。パスワードはプロンプトしない。セッションバスは `DBUS_SESSION_BUS_ADDRESS` か `/run/user/<uid>/bus` があるとあるものとして扱う
- `auth emergency-revoke` は Google が HTTP 200、または HTTP 400 で `error` が `invalid_token` のとき、ローカルトークンを削除してアカウントをブロックする。通信失敗・5xx・トークン無しでも `blocked_accounts` へ追加する。`--dry-run` は失効リクエストを送らない
- `gmail search` と `calendar list` の `--all-pages` は `--max-pages`（既定 10、1〜50）ページで止まる。続きがあるときは `nextPageToken` を返し、終了コードは 0。範囲外の `--max-pages` は `invalid_arguments`（終了コード 1）で、レート制限は消費しない

## Safety Controls

- `--dry-run`: API を呼ばずに実行内容を確認する
- `--confirm-*`: 削除・置換などの高リスク操作に必要
- `--approval-token`: 承認必須アクションで使う。既定の対象は `calendar.delete`、`docs.write.replace`、`docs.find_replace`、`sheets.update`、`slides.write`。`require_approval_actions` はこれに追加し、`exempt_approval_actions` は既定から外す。両方に書いたアクションは承認必須のまま
- 書き込み系コマンド（calendar の作成・更新・削除、docs の作成・書き込み・置換・export、sheets の update・append、slides write）は action ごとに 1 分あたり 30 回まで。超過時の code は `rate_limited`。`--dry-run` は回数に含めない。`gmail.send` は 1 分あたり 20 回
- 承認トークンはレート制限を通過したあとに消費する。`rate_limited` のときはトークンは残る
- `--audit-log`: 書き込み操作を JSONL で記録する。省略時は設定ディレクトリの `audit.log`。追記はファイルロックの中で行い、新規行の `hash` は keyring の HMAC-SHA256（`hash_alg=hmac-sha256`）。末尾ハッシュも keyring に残す。`hash_alg` の無い行は従来の SHA-256 として検証する
- `--allowed-output-dir`: エクスポートなどのファイル出力先を制限する。外への出力は `output_not_allowed`（終了コード 4）。`--dry-run` でも同じ

## 実行ポリシー（policy.json）

パスは設定ディレクトリの `policy.json`（Linux は `~/.config/gog-lite/policy.json`、macOS は `~/Library/Application Support/gog-lite/policy.json`）。ファイルが無いときはアクション制限はなく、既定の承認必須アクションだけが有効になる。

| フィールド | 意味 |
|---|---|
| `allowed_actions` | 空ならすべて許可。値があるときはそのアクションだけ許可 |
| `blocked_accounts` | 拒否するアカウント |
| `require_approval_actions` | 既定の承認必須アクションに追加する |
| `exempt_approval_actions` | 既定から承認を外す。`require_approval_actions` にもあるアクションは承認必須のまま |

既定の承認必須アクションは `calendar.delete`、`docs.write.replace`、`docs.find_replace`、`sheets.update`、`slides.write`。実行時にトークンを確認するのは `calendar delete`、`docs write --replace`、`docs find-replace`、`sheets update`、`slides write`。`auth approval-token` は、この実効集合に入っているアクションにだけトークンを発行する。

```json
{
  "require_approval_actions": ["gmail.draft"],
  "exempt_approval_actions": ["slides.write"]
}
```

この例では `gmail.draft` の承認トークンを発行でき、`slides.write` は承認不要になる。`calendar.delete` は承認必須のまま。`slides write` の `--confirm-write` は残る。

## What Agents May Safely Depend On

- この文書に記載されたトップレベルの CLI 契約
- 成功/失敗の JSON で公開している意味上のフィールド
- `gmail send` が「送信」ではなく「下書き保存」であること

次のものは長期的な統合面としては優先しない。

- Google API の raw payload の細かな shape
- 互換性を明記していない内部由来の詳細フィールド

## Command Response Shape

各コマンド節では、次の順に読む前提とする。

1. そのコマンドが何をするか
2. 必須前提や安全制御
3. 成功時の代表 JSON
4. 失敗時に見ればよい代表 code

---

## セットアップ

### 1. OAuth クライアント認証情報の配置

Google Cloud Console で「デスクトップアプリ」タイプの OAuth2 クライアントを作成し、
ダウンロードした JSON を以下に配置する（または `client_id`/`client_secret` を直接記述）：

```
# macOS (ファイル方式):
$HOME/Library/Application Support/gog-lite/credentials.json

# Linux (ファイル方式):
~/.config/gog-lite/credentials.json
```

External のテスト中アプリでは、OAuth 同意画面の Test users に利用アカウント追加が必要。
macOS では `GOG_LITE_CLIENT_ID` / `GOG_LITE_CLIENT_SECRET` を Keychain に保存して
`credentials.json` を削除する運用も可能。

### 2. アカウント認証（2ステップ）

**ステップ 1** — 認証 URL を取得：

```bash
gog-lite auth login --account you@gmail.com --services gmail,calendar,docs
```

```json
{
  "auth_url": "https://accounts.google.com/o/oauth2/auth?...",
  "next_step": "run again with --auth-url <redirect URL from browser>"
}
```

ブラウザで `auth_url` を開く。認証後、ブラウザのアドレスバーに
`http://127.0.0.1:PORT/oauth2/callback?code=...&state=...` のような URL が表示される。
ステップ 1 はそのポートを最大 10 分つかむので、コピーを案内するページが開くことがある。
ページが開かなくても、アドレスバーの URL をコピーする。

ステップ 1 は `auth_url` の JSON を出して終了する。ポートをつかみ続けるプロセスが最大 10 分残る。認可コードの交換には、状態ファイルに保存した PKCE `code_verifier` が要る。

すでに保存したトークンは、現在より広いスコープを持っている。同意の取り直しだけではスコープは減らない。Google アカウントのサードパーティ アクセスからこのクライアントを削除し、`auth remove` のあと `auth login --force-consent` でやり直す。手順は README の「スコープを狭めたあとの再認証」。

**ステップ 2** — コードを交換してトークンを保存：

```bash
gog-lite auth login --account you@gmail.com --services gmail,calendar,docs \
  --auth-url "http://127.0.0.1:PORT/oauth2/callback?code=4/0AX...&state=..."
```

```json
{
  "stored": true,
  "email": "you@gmail.com",
  "services": ["gmail", "calendar", "docs"]
}
```

### 3. 認証済みアカウント確認

```bash
gog-lite auth list
```

```json
{
  "accounts": [
    {
      "email": "you@gmail.com",
      "services": ["gmail", "calendar", "docs"],
      "created_at": "2026-02-25T10:00:00Z"
    }
  ]
}
```

---

## コマンドリファレンス

### auth

```bash
gog-lite auth login  --account EMAIL [--services gmail,calendar,docs] [--auth-url URL] [--force-consent]
gog-lite auth list
gog-lite auth remove --account EMAIL
gog-lite auth preflight --account EMAIL [--require-actions gmail.draft,calendar.create]
gog-lite auth approval-token --account EMAIL --action ACTION [--ttl 10m]
gog-lite auth emergency-revoke --account EMAIL
gog-lite --version
gog-lite --help
```

- `auth approval-token` は `ACTION` が policy で許可され、かつ承認必須（既定に `require_approval_actions` を足し、`exempt_approval_actions` で外した既定を除いた集合。両方にあるときは承認必須）である場合のみ発行される。
- `auth approval-token` の実行自体も `allowed_actions` の対象（`auth.approval_token`）。`allowed_actions` を設定している環境では明示的に追加が必要。
- `auth approval-token` は `--dry-run` に対応し、実トークンを発行せず検証できる。
- `auth emergency-revoke` は、ローカル削除の前に refresh token を `POST https://oauth2.googleapis.com/revoke`（`application/x-www-form-urlencoded` の `token`）へ送る。
- HTTP 200 のときはローカルトークンを削除し、policy の `blocked_accounts` にアカウントを加える。終了コードは 0。`already_revoked` は `false`。
- HTTP 400 かつ本文の `error` が `invalid_token` のときは、すでに失効済みとして同じ削除とブロックに進む。終了コードは 0。`already_revoked` は `true`。
- 通信失敗または HTTP 5xx のときは終了コード 1、`code` は `remote_revoke_failed`。`blocked_accounts` には追加する。refresh token は残す。`error` は「リモート失効に失敗した」で始まり、「ローカルではブロック済み」「再実行」を含む。
- `invalid_token` 以外の拒否（HTTP 400 の別コードなど）は終了コード 1、`code` は `remote_revoke_failed`。ローカル削除もブロックもしない。トークンは残る。
- 保存された refresh token が無いときは Google へ送らず、終了コード 3、`code` は `remote_revoke_failed`。`blocked_accounts` には追加する。
- `--dry-run` は失効リクエストを送らず、ローカル削除もブロックもしない。確認用 JSON と監査ログだけを返す。終了コードは 0。
- `auth remove` はローカルのトークン削除だけで、Google 側の失効はしない。
- `auth login` の userinfo 取得は 30 秒で打ち切る。時間切れの code は `auth_exchange_error`（終了コード 1）。

`auth emergency-revoke` の成功時（今回失効したとき）：

```json
{"revoked": true, "already_revoked": false, "blocked": true, "email": "you@gmail.com"}
```

すでに失効済み（HTTP 400 `invalid_token`）のとき：

```json
{"revoked": true, "already_revoked": true, "blocked": true, "email": "you@gmail.com"}
```

`--dry-run` のとき：

```json
{"dry_run": true, "action": "auth.emergency_revoke", "params": {"account": "you@gmail.com"}}
```

---

### gmail

#### search

```bash
gog-lite gmail search --account EMAIL --query QUERY [--max 20] [--all-pages] [--max-pages 10] [--page TOKEN]
```

```json
{
  "messages": [
    {"id": "18c3a...", "thread_id": "18c3a..."}
  ],
  "nextPageToken": "..."
}
```

`--max` は 1 ページあたりの件数。`--all-pages` は `nextPageToken` が空になるまで繰り返すが、`--max-pages`（既定 10、1〜50）で止まる。止めたときに続きがあれば、その `nextPageToken` を返す。続きは `--page` に渡す。`--max-pages` が範囲外のときは `invalid_arguments`（終了コード 1）。この失敗ではレート制限を消費しない。

よく使うクエリ例：
- `is:unread` — 未読
- `from:boss@example.com` — 送信者
- `subject:会議 after:2026/02/01` — 件名+日付
- `has:attachment` — 添付あり

#### get

```bash
gog-lite gmail get --account EMAIL --message-id ID [--format full|metadata|minimal|raw]
```

Gmail API の Message オブジェクト相当を返す。raw payload の詳細 shape は長期的な統合面としては推奨せず、
ID や終了コードなどの CLI 契約を優先して扱う。

#### send

メールを下書きとして保存する（送信はしない）。実際の送信は Gmail 上で手動で行う。

```bash
gog-lite gmail send --account EMAIL --to TO --subject SUBJECT [--body TEXT] [--body-stdin] [--cc CC] [--bcc BCC]
echo "本文" | gog-lite gmail send --account EMAIL --to TO --subject SUBJECT --body-stdin
```

```json
{"draft_id": "r123...", "message_id": "18c3a...", "thread_id": "18c3a...", "saved": true}
```

`--dry-run` では Gmail API を呼ばず、監査ログと確認用 JSON だけを返す。

```bash
gog-lite --dry-run gmail send --account you@gmail.com \
  --to boss@example.com --subject "週次レポート" --body "本文です"
```

```json
{
  "dry_run": true,
  "action": "gmail.draft",
  "params": {
    "account": "you@gmail.com",
    "to": "boss@example.com",
    "cc": "",
    "bcc": "",
    "subject": "週次レポート",
    "body_length": 12
  }
}
```

#### thread

```bash
gog-lite gmail thread --account EMAIL --thread-id ID [--format full|metadata|minimal]
```

#### labels

```bash
gog-lite gmail labels --account EMAIL
```

```json
{"labels": [{"id": "INBOX", "name": "受信トレイ", "type": "system"}, ...]}
```

---

### calendar

#### calendars

```bash
gog-lite calendar calendars --account EMAIL
```

```json
{"calendars": [{"id": "primary", "summary": "...", "primary": true, "access_role": "owner"}, ...]}
```

#### list

```bash
gog-lite calendar list --account EMAIL [--calendar-id primary] \
  [--from RFC3339] [--to RFC3339] [--max 20] [--all-pages] [--max-pages 10] [--page TOKEN] [--query TEXT]
```

```json
{
  "events": [
    {
      "id": "abc123",
      "summary": "チームMTG",
      "start": "2026-03-01T10:00:00+09:00",
      "end":   "2026-03-01T11:00:00+09:00",
      "location": "会議室A"
    }
  ],
  "nextPageToken": ""
}
```

`--max` は 1 ページあたりの件数。`--all-pages` は `--max-pages`（既定 10、最大 50）まで。上限に達して続きがあるときは `nextPageToken` を返す。範囲外の `--max-pages` は `invalid_arguments`（終了コード 1）。

時刻は RFC3339 形式（タイムゾーン必須）：`2026-03-01T00:00:00Z` / `2026-03-01T00:00:00+09:00`

#### get

```bash
gog-lite calendar get --account EMAIL --event-id ID [--calendar-id primary]
```

Calendar API の Event オブジェクト相当を返す。raw payload の詳細 shape ではなく、
CLI の終了コード・認証エラー・安全制御の契約に依存する前提で利用する。

#### create

```bash
gog-lite calendar create --account EMAIL --title TITLE --start RFC3339 --end RFC3339 \
  [--calendar-id primary] [--description TEXT] [--location TEXT]

# dry-run で確認
gog-lite --dry-run calendar create --account EMAIL --title "会議" \
  --start 2026-03-01T10:00:00Z --end 2026-03-01T11:00:00Z
```

```json
{"id": "abc123", "summary": "会議", "start": "...", "end": "...", "html_link": "https://calendar.google.com/..."}
```

#### update

```bash
gog-lite calendar update --account EMAIL --event-id ID \
  [--title TITLE] [--start RFC3339] [--end RFC3339] [--description TEXT] [--location TEXT]
```

#### delete

```bash
gog-lite calendar delete --account EMAIL --event-id ID [--calendar-id primary] --confirm-delete [--approval-token TOKEN]
```

```json
{"deleted": true, "event_id": "abc123"}
```

---

### docs

Doc ID は Google Docs の URL から取得：
`https://docs.google.com/document/d/**DOC_ID**/edit`

#### info

```bash
gog-lite docs info --account EMAIL --doc-id DOC_ID
```

```json
{"id": "...", "title": "無題のドキュメント", "revision_id": "..."}
```

#### cat

```bash
gog-lite docs cat --account EMAIL --doc-id DOC_ID [--max-bytes 2000000]
```

```json
{"id": "...", "title": "...", "content": "本文テキスト...", "truncated": false}
```

#### create

```bash
gog-lite docs create --account EMAIL --title TITLE [--content TEXT] [--content-stdin]
echo "初期内容" | gog-lite docs create --account EMAIL --title "新規ドキュメント" --content-stdin
```

```json
{"id": "...", "title": "新規ドキュメント", "url": "https://docs.google.com/document/d/.../edit"}
```

#### export

`--allowed-output-dir` の外は `output_not_allowed`（終了コード 4）。`--dry-run` でも同じで、API は呼ばない。

```bash
gog-lite docs export --account EMAIL --doc-id DOC_ID --format pdf|docx|txt|odt|html --output PATH [--overwrite]
```

```json
{"exported": true, "doc_id": "...", "format": "pdf", "output": "/tmp/doc.pdf", "bytes_written": 12345}
```

#### write

既存内容への挿入、または `--replace` による全置換。

`--replace` の本実行には `--confirm-replace` が必要。`docs.write.replace` は既定の承認必須アクションなので、`exempt_approval_actions` に含めていない限り `--approval-token` も必要。`--dry-run` では確認フラグも承認トークンも不要で、API は呼ばない。

```bash
gog-lite --dry-run docs write --account EMAIL --doc-id DOC_ID --content "新しい内容" --replace

gog-lite docs write --account EMAIL --doc-id DOC_ID [--content TEXT] [--content-stdin] [--replace --confirm-replace --approval-token TOKEN]

cat report.txt | gog-lite docs write --account EMAIL --doc-id DOC_ID --content-stdin --replace --confirm-replace --approval-token TOKEN
```

dry-run の出力：

```json
{
  "dry_run": true,
  "action": "docs.write",
  "params": {
    "account": "you@gmail.com",
    "doc_id": "...",
    "content_length": 15,
    "replace": true,
    "confirm_replace": false
  }
}
```

#### find-replace

```bash
gog-lite docs find-replace --account EMAIL --doc-id DOC_ID --find TEXT --replace TEXT [--match-case] --confirm-find-replace [--approval-token TOKEN]
```

```json
{"replaced": true, "doc_id": "...", "find": "旧", "replace": "新", "occurrences": 3}
```

---

### sheets

Spreadsheet ID は Google Sheets の URL から取得：
`https://docs.google.com/spreadsheets/d/**SPREADSHEET_ID**/edit`

#### info

```bash
gog-lite sheets info --account EMAIL --spreadsheet-id SPREADSHEET_ID
```

```json
{
  "spreadsheet_id": "...",
  "title": "売上データ",
  "url": "https://docs.google.com/spreadsheets/d/.../edit",
  "sheets": [
    {"sheet_id": 0, "title": "Sheet1", "row_count": 1000, "column_count": 26}
  ]
}
```

#### get

```bash
gog-lite sheets get --account EMAIL --spreadsheet-id SPREADSHEET_ID --range "Sheet1!A1:C10"
```

```json
{
  "spreadsheet_id": "...",
  "range": "Sheet1!A1:C3",
  "values": [["Name", "Age"], ["Alice", "30"]]
}
```

#### update

既存セルの上書き。値の既定は `RAW`（`=SUM(...)` のような文字列は数式にならない）。数式や日付として解釈するときだけ `--value-input USER_ENTERED` を付ける。

本実行には `--confirm-write` が必要。`sheets.update` は既定の承認必須アクションなので、`exempt_approval_actions` に含めていない限り `--approval-token` も必要。`require_approval_actions` に同じアクションがあるときは承認必須のまま。`--dry-run` では確認フラグも承認トークンも不要で、API は呼ばない。

```bash
gog-lite --dry-run sheets update --account EMAIL --spreadsheet-id SPREADSHEET_ID \
  --range "Sheet1!A1:B1" --values '[["Alice",30]]'

gog-lite sheets update --account EMAIL --spreadsheet-id SPREADSHEET_ID \
  --range "Sheet1!A1:B1" --values '[["Alice",30]]' \
  --confirm-write --approval-token TOKEN

# 数式として書き込む
gog-lite sheets update --account EMAIL --spreadsheet-id SPREADSHEET_ID \
  --range "Sheet1!C1" --values '[["=SUM(A1:A3)"]]' \
  --value-input USER_ENTERED --confirm-write --approval-token TOKEN
```

dry-run の出力：

```json
{
  "dry_run": true,
  "action": "sheets.update",
  "params": {
    "account": "you@gmail.com",
    "spreadsheet_id": "...",
    "range": "Sheet1!A1:B1",
    "row_count": 1,
    "value_input": "RAW"
  }
}
```

成功時：

```json
{
  "spreadsheet_id": "...",
  "updated_range": "Sheet1!A1:B1",
  "updated_rows": 1,
  "updated_columns": 2,
  "updated_cells": 2
}
```

`--confirm-write` が無いときの code は `write_requires_confirmation`（終了コード 1）。承認トークンが無い・不一致・使用済みのときの code は `approval_required`（終了コード 4）。

#### append

行を末尾に追加する。確認フラグは不要。値の既定は `RAW`。数式として追加するときだけ `--value-input USER_ENTERED` を付ける。

```bash
gog-lite sheets append --account EMAIL --spreadsheet-id SPREADSHEET_ID \
  --range Sheet1 --values '[["Bob",25]]'

# stdin から読み込み
echo '[["Bob",25]]' | gog-lite sheets append --account EMAIL \
  --spreadsheet-id SPREADSHEET_ID --range Sheet1 --values-stdin

# 数式として追加
gog-lite sheets append --account EMAIL --spreadsheet-id SPREADSHEET_ID \
  --range Sheet1 --values '[["=SUM(A1:A3)"]]' --value-input USER_ENTERED
```

```json
{
  "spreadsheet_id": "...",
  "table_range": "Sheet1!A1:B5",
  "updated_range": "Sheet1!A6:B6",
  "updated_rows": 1
}
```

---

### slides

Presentation ID は Google Slides の URL から取得：
`https://docs.google.com/presentation/d/**PRESENTATION_ID**/edit`

#### info

```bash
gog-lite slides info --account EMAIL --presentation-id PRESENTATION_ID
```

```json
{
  "presentation_id": "...",
  "title": "Q1 レポート",
  "url": "https://docs.google.com/presentation/d/.../edit",
  "slide_count": 5,
  "slides": [
    {"object_id": "p1", "slide_number": 1},
    {"object_id": "p2", "slide_number": 2}
  ]
}
```

#### get

```bash
# 全スライドのテキストを取得
gog-lite slides get --account EMAIL --presentation-id PRESENTATION_ID

# 特定スライドのみ
gog-lite slides get --account EMAIL --presentation-id PRESENTATION_ID --page-id p1
```

全スライド取得時の出力：
```json
{
  "presentation_id": "...",
  "slides": [
    {"object_id": "p1", "slide_number": 1, "texts": ["タイトル", "サブタイトル"]},
    {"object_id": "p2", "slide_number": 2, "texts": ["本文"]}
  ]
}
```

page-id 指定時の出力：
```json
{"object_id": "p1", "texts": ["タイトル", "サブタイトル"]}
```

#### write

テキストの検索・置換（`ReplaceAllText`）を行う。

```bash
gog-lite slides write --account EMAIL --presentation-id PRESENTATION_ID \
  --find "{{NAME}}" --replace "Alice" --confirm-write [--approval-token TOKEN]

# dry-run で確認
gog-lite --dry-run slides write --account EMAIL --presentation-id PRESENTATION_ID \
  --find "{{NAME}}" --replace "Alice"
```

```json
{"presentation_id": "...", "occurrences_changed": 3}
```

---

## エラー処理

全エラーは **stderr に JSON**、**stdout は空**、**終了コード != 0**。

```json
{"error": "エラーの説明", "code": "error_code_string"}
```

| code | 終了コード | 意味 |
|------|-----------|------|
| `auth_required` | 2 | トークン未保存、または Google API が HTTP 401 を返した。どちらも `auth login` が必要 |
| `remote_revoke_failed` | 1 | 通信失敗または HTTP 5xx。`blocked_accounts` に追加済み。refresh token は残る。再実行する |
| `remote_revoke_failed` | 1 | `invalid_token` 以外の拒否。ローカル削除もブロックもしない |
| `remote_revoke_failed` | 3 | 保存された refresh token がない。Google へは送らない。`blocked_accounts` には追加する |
| `credentials_missing` | 1 | `credentials.json` がない |
| `invalid_time` | 1 | RFC3339 フォーマットエラー |
| `invalid_arguments` | 1 | 必須引数の不足や不正なフラグ。`--value-input` が `RAW` / `USER_ENTERED` 以外、`--max-pages` が 1〜50 以外のときもこれ |
| `write_requires_confirmation` | 1 | 上書きに `--confirm-write` がない（`sheets update` など） |
| `replace_requires_confirmation` | 1 | `docs write --replace` に `--confirm-replace` がない。`--dry-run` では不要 |
| `output_not_allowed` | 4 | `--allowed-output-dir` の外へ `docs export` しようとした。`--dry-run` でも同じ |
| `approval_required` | 4 | 承認必須アクションで token がない・不一致・使用済み・期限切れ |
| `rate_limited` | 1 | action のレート制限を超えた |
| `search_error` / `get_error` 等 | 1 | API エラー。HTTP 401 は `auth_required`（終了コード 2） |

### 認証エラーの検出パターン

終了コード 2 は、トークンが保存されていないときと、Google API が HTTP 401 を返したときの両方です。`code` はどちらも `auth_required` です。エージェントは `auth login` に進みます。

```bash
err_json=$(mktemp)
output=$(gog-lite gmail labels --account you@gmail.com 2>"$err_json")
exit_code=$?
if [ $exit_code -eq 2 ]; then
  echo "要認証: $(jq -r .error "$err_json")"
fi
```

---

## dry-run

`--dry-run`（`-n`）は書き込み前の標準確認手段として扱う。
自動化では、本実行前に dry-run を使って引数・安全制御・出力形を確認する前提で運用する。

```bash
gog-lite --dry-run calendar create --account EMAIL \
  --title "テスト会議" --start 2026-03-01T10:00:00Z --end 2026-03-01T11:00:00Z
```

```json
{
  "dry_run": true,
  "action": "calendar.create",
  "params": {
    "account": "you@gmail.com",
    "calendar_id": "primary",
    "title": "テスト会議",
    "start": "2026-03-01T10:00:00Z",
    "end": "2026-03-01T11:00:00Z",
    "description": "",
    "location": ""
  }
}
```

---

## stdin パイプ

`--body-stdin`（gmail send）と `--content-stdin`（docs create/write）で本文をパイプ渡しできる。

```bash
# ファイル内容をメール送信
cat report.md | gog-lite gmail send \
  --account you@gmail.com --to boss@example.com --subject "週次レポート" --body-stdin

# LLM 生成テキストをドキュメントに書き込み
generate_text | gog-lite docs write \
  --account you@gmail.com --doc-id DOC_ID --content-stdin --replace --confirm-replace --approval-token TOKEN
```

---

## 環境変数

| 変数 | 説明 |
|------|------|
| `GOG_LITE_CLIENT_ID` | OAuth クライアント ID（credentials.json より優先） |
| `GOG_LITE_CLIENT_SECRET` | OAuth クライアントシークレット（credentials.json より優先） |
| `GOG_LITE_KEYRING_BACKEND` | `keychain`（macOS）/ `file`（ヘッドレス）。未設定かつ Linux でセッションバスが無いときも `file` |
| `GOG_LITE_KEYRING_PASSWORD` | ファイルバックエンドの暗号化パスワード。空や空白だけは不可 |

ヘッドレス環境（Docker/CI）での完全な設定：
```bash
export GOG_LITE_CLIENT_ID=xxxx.apps.googleusercontent.com
export GOG_LITE_CLIENT_SECRET=GOCSPX-xxxx
export GOG_LITE_KEYRING_BACKEND=file
export GOG_LITE_KEYRING_PASSWORD=your-secure-password
```

参照優先順位は次の通り：
1. `GOG_LITE_CLIENT_ID` と `GOG_LITE_CLIENT_SECRET`（両方）
2. macOS の場合は Keychain（`GOG_LITE_CLIENT_ID` / `GOG_LITE_CLIENT_SECRET`）
3. `credentials.json`

片方のみ設定されている場合は次の経路へフォールバックする。
ファイルバックエンドでは `GOG_LITE_KEYRING_PASSWORD` が必須。`GOG_LITE_KEYRING_BACKEND=file` を指定したときと、Linux でセッションバスが無くファイルバックエンドへ自動で切り替わるときの両方。未設定、空文字、空白だけは `keyring_error`（終了コード 1）。端末へのパスワード入力は行わない。セッションバスは、`DBUS_SESSION_BUS_ADDRESS` か `/run/user/<uid>/bus` があるとあるものとして扱う。

---

## ページネーション

`--max` は 1 ページあたりの件数。`--all-pages` はそれを `--max-pages` ページまで繰り返す。既定は 10、最大は 50。上限に達して続きがあるときは、取得済みの結果と `nextPageToken` を返して止まる（終了コード 0）。メールボックス全体を 1 回の実行に載せない。

```bash
# 最新20件だけ
gog-lite gmail search --account EMAIL --query "is:unread" --max 20

# 最大10ページ（--max-pages の既定）。続きがあれば nextPageToken が返る
gog-lite gmail search --account EMAIL --query "is:unread" --all-pages

# ページ数を上げる（1〜50）
gog-lite gmail search --account EMAIL --query "is:unread" --all-pages --max-pages 50

# 手動ページング（nextPageToken を次の --page に渡す）
first=$(gog-lite gmail search --account EMAIL --query "label:inbox" --max 10)
token=$(echo "$first" | jq -r .nextPageToken)
gog-lite gmail search --account EMAIL --query "label:inbox" --max 10 --page "$token"

# 上限で止まった続きも、同じトークンを --page に渡す
gog-lite gmail search --account EMAIL --query "is:unread" --all-pages --page "$token"
```
