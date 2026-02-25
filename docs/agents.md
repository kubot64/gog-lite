# gog-lite — AI Agent Reference

AIエージェントが gog-lite を使う際に必要な情報をまとめたリファレンス。

---

## 基本原則

| 原則 | 内容 |
|------|------|
| stdout | 常に JSON（色・表・TSVなし） |
| stderr | エラーのみ `{"error": "...", "code": "..."}` |
| 終了コード | 0=成功 / 1=エラー / 2=認証エラー / 3=未発見 / 4=権限なし |
| 認証 | ブラウザ不要。2ステップで URL → コード交換 |
| `--account` | 各コマンドの必須フラグ（グローバルではない） |
| `--dry-run` | グローバルフラグ。API呼び出しなしで実行内容を確認 |
| `--audit-log` | グローバルフラグ。書き込み系操作の監査ログ(JSONL) |
| `--allowed-output-dir` | グローバルフラグ。ファイル出力先ディレクトリ制限 |

---

## セットアップ

### 1. OAuth クライアント認証情報の配置

Google Cloud Console で「デスクトップアプリ」タイプの OAuth2 クライアントを作成し、
ダウンロードした JSON を以下に配置する（または `client_id`/`client_secret` を直接記述）：

```
~/.config/gog-lite/credentials.json
```

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
`http://127.0.0.1:PORT/oauth2/callback?code=...&state=...` のような URL が表示される
（ページは読み込めなくてよい）。そのURLをコピーする。

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
gog-lite auth preflight --account EMAIL [--require-actions gmail.send,calendar.create]
gog-lite auth approval-token --account EMAIL --action ACTION [--ttl 10m]
gog-lite auth emergency-revoke --account EMAIL
```

---

### gmail

#### search

```bash
gog-lite gmail search --account EMAIL --query QUERY [--max 20] [--all-pages] [--page TOKEN]
```

```json
{
  "messages": [
    {"id": "18c3a...", "thread_id": "18c3a..."}
  ],
  "nextPageToken": "..."
}
```

よく使うクエリ例：
- `is:unread` — 未読
- `from:boss@example.com` — 送信者
- `subject:会議 after:2026/02/01` — 件名+日付
- `has:attachment` — 添付あり

#### get

```bash
gog-lite gmail get --account EMAIL --message-id ID [--format full|metadata|minimal|raw]
```

生メッセージ（Gmail API の Message オブジェクト）をそのまま返す。

#### send

```bash
gog-lite gmail send --account EMAIL --to TO --subject SUBJECT [--body TEXT] [--body-stdin] [--cc CC] [--bcc BCC]
echo "本文" | gog-lite gmail send --account EMAIL --to TO --subject SUBJECT --body-stdin
```

```json
{"id": "18c3a...", "thread_id": "18c3a...", "sent": true}
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
  [--from RFC3339] [--to RFC3339] [--max 20] [--all-pages] [--page TOKEN] [--query TEXT]
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

時刻は RFC3339 形式（タイムゾーン必須）：`2026-03-01T00:00:00Z` / `2026-03-01T00:00:00+09:00`

#### get

```bash
gog-lite calendar get --account EMAIL --event-id ID [--calendar-id primary]
```

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

```bash
gog-lite docs export --account EMAIL --doc-id DOC_ID --format pdf|docx|txt|odt|html --output PATH [--overwrite]
```

```json
{"exported": true, "doc_id": "...", "format": "pdf", "output": "/tmp/doc.pdf", "bytes_written": 12345}
```

#### write

```bash
gog-lite docs write --account EMAIL --doc-id DOC_ID [--content TEXT] [--content-stdin] [--replace --confirm-replace --approval-token TOKEN]
# --replace で既存内容をすべて置換
cat report.txt | gog-lite docs write --account EMAIL --doc-id DOC_ID --content-stdin --replace
```

#### find-replace

```bash
gog-lite docs find-replace --account EMAIL --doc-id DOC_ID --find TEXT --replace TEXT [--match-case] --confirm-find-replace [--approval-token TOKEN]
```

```json
{"replaced": true, "doc_id": "...", "find": "旧", "replace": "新", "occurrences": 3}
```

---

## エラー処理

全エラーは **stderr に JSON**、**stdout は空**、**終了コード != 0**。

```json
{"error": "エラーの説明", "code": "error_code_string"}
```

| code | 終了コード | 意味 |
|------|-----------|------|
| `auth_required` | 2 | トークン未保存（`auth login` が必要） |
| `credentials_missing` | 1 | `credentials.json` がない |
| `invalid_time` | 1 | RFC3339 フォーマットエラー |
| `search_error` / `get_error` 等 | 1 | API エラー |

### 認証エラーの検出パターン

```bash
output=$(gog-lite gmail labels --account you@gmail.com 2>/tmp/err.json)
exit_code=$?
if [ $exit_code -eq 2 ]; then
  echo "要認証: $(cat /tmp/err.json | jq -r .error)"
fi
```

---

## dry-run

書き込み系コマンド（`calendar create/update/delete`, `docs write/find-replace`）は
グローバルフラグ `--dry-run`（`-n`）で実際には実行せずに確認できる。

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
  --account you@gmail.com --doc-id DOC_ID --content-stdin --replace
```

---

## 環境変数

| 変数 | 説明 |
|------|------|
| `GOG_LITE_CLIENT_ID` | OAuth クライアント ID（credentials.json より優先） |
| `GOG_LITE_CLIENT_SECRET` | OAuth クライアントシークレット（credentials.json より優先） |
| `GOG_LITE_KEYRING_BACKEND` | `keychain`（macOS）/ `file`（ヘッドレス） |
| `GOG_LITE_KEYRING_PASSWORD` | ファイルバックエンド使用時のパスワード |

ヘッドレス環境（Docker/CI）での完全な設定：
```bash
export GOG_LITE_CLIENT_ID=xxxx.apps.googleusercontent.com
export GOG_LITE_CLIENT_SECRET=GOCSPX-xxxx
export GOG_LITE_KEYRING_BACKEND=file
export GOG_LITE_KEYRING_PASSWORD=your-secure-password
```

`GOG_LITE_CLIENT_ID` と `GOG_LITE_CLIENT_SECRET` の**両方**が設定されている場合、credentials.json は参照されない。
片方のみの場合はファイルにフォールバックする。
`GOG_LITE_KEYRING_BACKEND=file` の場合、`GOG_LITE_KEYRING_PASSWORD` は必須。

---

## ページネーション

`--max` と `--all-pages` を使い分ける：

```bash
# 最新20件だけ
gog-lite gmail search --account EMAIL --query "is:unread" --max 20

# 全件取得（注意：大量になる場合あり）
gog-lite gmail search --account EMAIL --query "is:unread" --all-pages

# 手動ページング（nextPageToken を次の --page に渡す）
first=$(gog-lite gmail search --account EMAIL --query "label:inbox" --max 10)
token=$(echo "$first" | jq -r .nextPageToken)
gog-lite gmail search --account EMAIL --query "label:inbox" --max 10 --page "$token"
```
