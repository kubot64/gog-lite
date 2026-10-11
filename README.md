# gog-lite

AIエージェントが Gmail / Google Calendar / Google Docs を操作するためのシンプルな CLI。

[gogcli](https://github.com/steipete/gogcli) の多機能さをAIエージェント向けに絞り込んだ派生版。

## Public Contract

- **stdout は常に JSON** — `--help` / `--version` を含め、色・表・TSV は出力しない
- **stderr は常に JSON エラー** — `{"error": "...", "code": "..."}` を返し、stdout と混在しない
- **終了コードは固定** — `0=成功 / 1=エラー / 2=認証エラー / 3=未発見 / 4=権限なし`。終了コード 2 の `auth_required` は、トークンが無いときと、Google API が HTTP 401 を返したとき
- **破壊的操作には安全制御がある** — `confirm` フラグと、必要に応じて `approval-token` を要求する
- **Sheets のセル値は既定で RAW** — `=` で始まる文字列を数式にしない。数式にするときだけ `--value-input USER_ENTERED`。`sheets update` は `--confirm-write` と approval-token が必要
- **`--dry-run` は書き込み前の標準確認手段** — 実行前に API 呼び出しなしで内容確認できる。`docs write --replace` では `--confirm-replace` も承認トークンも要らない
- **`gmail send` は下書き保存契約** — 即時送信ではなく Gmail draft として保存する
- **`--all-pages` はページ数に上限がある** — `gmail search` と `calendar list` の `--max-pages` は既定 10、最大 50。上限で打ち切ったときは `nextPageToken` を返す

## Safety Controls

- `--dry-run` — 書き込み前の事前確認
- `--audit-log` — 書き込み操作の JSONL 監査ログ。新規行は keyring の HMAC 鍵で鎖をつなぐ
- `--allowed-output-dir` — ファイル出力先ディレクトリ制限。外への `docs export` は終了コード 4（`output_not_allowed`）。`--dry-run` でも同じ
- `--confirm-*` — 破壊的操作の明示確認
- `--approval-token` — 高リスク操作の追加承認。既定の対象は `calendar.delete`、`docs.write.replace`、`docs.find_replace`、`sheets.update`、`slides.write`
- ファイルロック — 承認トークンの消費、書き込みのレート制限、監査ログの追記は、ロックを持っているプロセスが終了するまで排他する。5 秒以内に取れないとき、監査ログは終了コード 1（`audit_error`）、レート制限の更新は終了コード 1（`rate_limited`）、承認トークンの消費は終了コード 4（`approval_required`）。`error` には `acquire lock` と `timeout` が入る。この失敗では承認トークンは消費しない

## 実行ポリシー（policy.json）

設定ディレクトリの `policy.json`（Linux は `~/.config/gog-lite/policy.json`、macOS は `~/Library/Application Support/gog-lite/policy.json`）で実行を制限できる。ファイルが無いときはアクション制限はなく、上の既定だけが承認必須になる。

- `allowed_actions`: 空ならすべて許可する。値があるときは、そのアクションだけ許可する。
- `blocked_accounts`: 拒否するアカウント。
- `require_approval_actions`: ここに書いたアクションを、既定の承認必須アクションに加える。
- `exempt_approval_actions`: 既定の対象から承認を外す。`require_approval_actions` にも書いてあるアクションは承認必須のまま。

承認を確認するのは `calendar delete`、`docs write --replace`、`docs find-replace`、`sheets update`、`slides write` と、`auth approval-token` の発行である。

```json
{
  "require_approval_actions": ["gmail.draft"],
  "exempt_approval_actions": ["slides.write"]
}
```

この例では `auth approval-token --action gmail.draft` を発行でき、`slides write` は承認トークンなしで実行できる。`calendar.delete` など残りの既定は承認必須のまま。`slides write` の `--confirm-write` は別の確認で、オプトアウトしても残る。

## インストール

### Homebrew（macOS / Linux）

```bash
brew tap kubot64/gog-lite https://github.com/kubot64/gog-lite
brew install gog-lite
```

アップグレード:

```bash
brew update
brew upgrade gog-lite
```

### ソースからビルド

```bash
git clone https://github.com/kubot64/gog-lite
cd gog-lite
go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" -o ~/bin/gog-lite ./cmd/gog-lite/
```

タグがない開発ブランチでは `--version` は `dev`（または `git describe` の値）になります。

## バージョン運用

- CalVer `vYYYY.MMDD.HHmm`（例: `v2026.0226.1430`）を採用。
- タグ生成とリリースを分離（Tag Release → Release）。
- まずタグ生成ワークフローを実行:

```bash
gh workflow run tag.yml --ref main
```

- タグ push をトリガーに Release ワークフローが自動実行される。
- `--version` の値はビルド時の `-ldflags "-X main.version=..."` で注入する。
- ローカルビルドでは `dev` がデフォルト:

```bash
go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" -o ~/bin/gog-lite ./cmd/gog-lite/
gog-lite --version
# -> {"version":"v2026.0227.0349"} などのJSON
```

## 開発時のワークフロー検証

GitHub Actions の YAML 構文と workflow 設定を事前に確認できます。

```bash
./scripts/check-workflows.sh
./scripts/check-action-refs.sh
```

`check-action-refs.sh` は、外部アクションの SHA が解決できることに加え、`# v6` のようなコメントがそのコミットのタグ（`v6` または `v6.0.2`）と一致するかを確認します。

`actionlint` をインストール済みの場合は、構文チェックに加えて workflow lint も実行します。

```bash
brew install actionlint
```

## セットアップ

### 1. OAuth クライアント認証情報

[Google Cloud Console](https://console.cloud.google.com/) で以下を行う：

1. プロジェクトを作成（または既存を使用）
2. 必要なAPIを有効化：Gmail API / Google Calendar API / Google Docs API / Google Drive API
3. 「Google Auth Platform」→「OAuth 同意画面（Audience）」でアプリ情報を入力
4. 外部ユーザー（External）でテスト中の場合は「Test users」に利用アカウントを追加
5. 「認証情報」→「OAuthクライアントID」を作成（種類：**デスクトップアプリ**）
6. JSONをダウンロードして一時配置（`os.UserConfigDir()` 配下）：

```bash
# macOS
mkdir -p "$HOME/Library/Application Support/gog-lite"
cp ~/Downloads/client_secret_*.json "$HOME/Library/Application Support/gog-lite/credentials.json"

# Linux
mkdir -p ~/.config/gog-lite
cp ~/Downloads/client_secret_*.json ~/.config/gog-lite/credentials.json
```

7. （macOS推奨）`client_id` / `client_secret` を Keychain へ移す：

```bash
security add-generic-password -a "$USER" -s GOG_LITE_CLIENT_ID -w '<YOUR_CLIENT_ID>' -U
security add-generic-password -a "$USER" -s GOG_LITE_CLIENT_SECRET -w '<YOUR_CLIENT_SECRET>' -U
```

8. Keychain 登録後は `credentials.json` を削除してよい（環境変数と Keychain を優先参照）：

```bash
# macOS
rm "$HOME/Library/Application Support/gog-lite/credentials.json"

# Linux
rm ~/.config/gog-lite/credentials.json
```

### 2. アカウント認証（2ステップ）

```bash
# ステップ1: 認証URLを取得
gog-lite auth login --account you@gmail.com --services gmail,calendar,docs
# → {"auth_url": "https://accounts.google.com/...", "next_step": "..."}
```

ブラウザで `auth_url` を開いて認証する。認証後、ブラウザは `http://127.0.0.1:PORT/oauth2/callback?code=...` に戻る。ステップ 1 はそのポートを最大 10 分つかんだままにするので、URL のコピーを案内するページが開く。ページが開かない場合でも、アドレスバーの URL をコピーすればよい。

ステップ 1 のコマンド自体は `auth_url` を出力して終了する。ポートをつかみ続ける別プロセスが最大 10 分残る。

> `エラー 403: access_denied` が出る場合は、OAuth 同意画面の Test users に該当 Gmail を追加してください。

```bash
# ステップ2: リダイレクトURLを渡してトークンを保存
gog-lite auth login --account you@gmail.com --services gmail,calendar,docs \
  --auth-url "http://127.0.0.1:PORT/oauth2/callback?code=..."
# → {"stored": true, "email": "you@gmail.com", "services": [...]}
```

### スコープを狭めたあとの再認証

ログイン時に同意する OAuth スコープは、各コマンドが実際に使う範囲だけになりました。それ以前に保存した refresh token は、より広い権限（Gmail の変更・設定、Drive の全ファイルの変更など）を持ったまま残ります。同意 URL には `include_granted_scopes=true` を付けるため、同意画面を出し直すだけでは、過去に付いたスコープは外れません。

次の順でやり直してください。

1. ブラウザで [Google アカウントのサードパーティ アクセス](https://myaccount.google.com/permissions) を開き、この OAuth クライアント（gog-lite）のアクセス権を削除する。
2. ローカルに保存したトークンを削除する。

```bash
gog-lite auth remove --account you@gmail.com
```

3. `--force-consent` を付けて同意を取り直す。

```bash
gog-lite auth login --account you@gmail.com --services gmail,calendar,docs --force-consent
```

4. 表示された `auth_url` をブラウザで開き、リダイレクト先の URL を `--auth-url` に渡して、上のステップ 2 を完了する。

新しく保存されるトークンが持つスコープは、選んだサービスについて下の表だけです。再認証の前は、API 呼び出しが `auth_required` で失敗することがあります。

## 使い方

### 認証管理

```bash
gog-lite auth list                        # 認証済みアカウント一覧
gog-lite auth remove --account EMAIL      # アカウントのトークンを削除
gog-lite auth preflight --account EMAIL --require-actions gmail.draft,calendar.create
gog-lite auth approval-token --account EMAIL --action calendar.delete --ttl 10m
gog-lite auth emergency-revoke --account EMAIL  # Google で refresh token を失効してから削除・ブロック
```

`auth emergency-revoke` は、ローカルのトークンを消す前に refresh token を `https://oauth2.googleapis.com/revoke` へ POST する。

Google が HTTP 200 を返すと、ローカルのトークンを削除し、policy の `blocked_accounts` にアカウントを追加する。終了コードは 0。

```json
{"revoked": true, "already_revoked": false, "blocked": true, "email": "you@gmail.com"}
```

HTTP 400 で `error` が `invalid_token` のとき（すでに無効、または失効直後の再実行）も失効済みとして同じ削除とブロックに進む。終了コードは 0。`already_revoked` は `true`。

通信失敗または HTTP 5xx のときは、終了コード 1、`code` は `remote_revoke_failed`。`blocked_accounts` には追加し、refresh token は再実行用に残す。`error` は「リモート失効に失敗した」で始まり、ローカルではブロック済みで再実行できることを示す。`invalid_token` 以外の拒否は終了コード 1 で、ローカル削除もブロックもしない。

保存された refresh token が無いときは Google へ送らず、終了コード 3、同じ `code`。この場合も `blocked_accounts` には追加する。

`--dry-run` は失効リクエストを送らない。確認用 JSON と監査ログだけを返し、削除もブロックもしない。

```json
{"dry_run": true, "action": "auth.emergency_revoke", "params": {"account": "you@gmail.com"}}
```

`auth remove` はローカルのトークンを削除するだけで、Google 側は失効しない。

### Gmail

```bash
# 未読メールを検索（--max は 1 ページあたりの件数）
gog-lite gmail search --account you@gmail.com --query "is:unread" --max 10

# 続きをまとめて取る（既定 10 ページ、最大 50）。上限で止めたときは nextPageToken が残る
gog-lite gmail search --account you@gmail.com --query "is:unread" --all-pages --max-pages 10

# メール本文を取得
gog-lite gmail get --account you@gmail.com --message-id MESSAGE_ID

# 保存前に内容を確認（API は呼ばない）
gog-lite --dry-run gmail send --account you@gmail.com \
  --to boss@example.com --subject "週次レポート" --body "本文です"

# メールを下書きとして保存（送信はしない）
gog-lite gmail send --account you@gmail.com \
  --to boss@example.com --subject "週次レポート" --body "本文です"

# パイプで本文を渡す
cat report.txt | gog-lite gmail send --account you@gmail.com \
  --to boss@example.com --subject "レポート" --body-stdin

# スレッド取得・ラベル一覧
gog-lite gmail thread --account you@gmail.com --thread-id THREAD_ID
gog-lite gmail labels --account you@gmail.com
```

### Google Calendar

```bash
# カレンダー一覧
gog-lite calendar calendars --account you@gmail.com

# 今週のイベントを取得（--all-pages も --max-pages 既定 10、最大 50）
gog-lite calendar list --account you@gmail.com \
  --from 2026-02-24T00:00:00Z --to 2026-03-02T23:59:59Z

# イベントを作成（--dry-run で確認してから）
gog-lite --dry-run calendar create --account you@gmail.com \
  --title "チームMTG" --start 2026-03-01T10:00:00+09:00 --end 2026-03-01T11:00:00+09:00

gog-lite calendar create --account you@gmail.com \
  --title "チームMTG" --start 2026-03-01T10:00:00+09:00 --end 2026-03-01T11:00:00+09:00

# イベントの更新・削除
gog-lite calendar update --account you@gmail.com --event-id EVENT_ID --title "新しいタイトル"
gog-lite calendar delete --account you@gmail.com --event-id EVENT_ID --confirm-delete --approval-token TOKEN
```

> 時刻は RFC3339 形式でタイムゾーン必須：`2026-03-01T10:00:00Z` または `2026-03-01T10:00:00+09:00`

### Google Docs

```bash
# ドキュメント情報・本文取得
gog-lite docs info --account you@gmail.com --doc-id DOC_ID
gog-lite docs cat  --account you@gmail.com --doc-id DOC_ID

# 新規作成
gog-lite docs create --account you@gmail.com --title "新しいドキュメント"

# 内容の書き込み（--replace で全置換。--dry-run では確認フラグと承認トークンは不要）
gog-lite --dry-run docs write --account you@gmail.com --doc-id DOC_ID --content "新しい内容" --replace

gog-lite docs write --account you@gmail.com --doc-id DOC_ID --content "新しい内容" --replace --confirm-replace --approval-token TOKEN

# エクスポート
gog-lite docs export --account you@gmail.com --doc-id DOC_ID --format pdf --output ~/Downloads/doc.pdf --overwrite

# テキスト置換
gog-lite docs find-replace --account you@gmail.com --doc-id DOC_ID --find "旧文言" --replace "新文言" --confirm-find-replace --approval-token TOKEN
```

### Google Sheets

```bash
# スプレッドシート情報を取得
gog-lite sheets info --account you@gmail.com --spreadsheet-id SPREADSHEET_ID

# セル範囲の値を取得
gog-lite sheets get --account you@gmail.com --spreadsheet-id SPREADSHEET_ID \
  --range "Sheet1!A1:C5"

# セル値を更新（既定は RAW。上書きには --confirm-write と approval-token が必要）
gog-lite --dry-run sheets update --account you@gmail.com --spreadsheet-id SPREADSHEET_ID \
  --range "Sheet1!A1:B1" --values '[["Alice",30]]'

gog-lite sheets update --account you@gmail.com --spreadsheet-id SPREADSHEET_ID \
  --range "Sheet1!A1:B1" --values '[["Alice",30]]' \
  --confirm-write --approval-token TOKEN

# 数式として書き込むときだけ USER_ENTERED
gog-lite sheets update --account you@gmail.com --spreadsheet-id SPREADSHEET_ID \
  --range "Sheet1!C1" --values '[["=SUM(A1:A3)"]]' \
  --value-input USER_ENTERED --confirm-write --approval-token TOKEN

# 行を末尾に追加（stdin から。既定は RAW）
echo '[["Bob",25]]' | gog-lite sheets append --account you@gmail.com \
  --spreadsheet-id SPREADSHEET_ID --range Sheet1 --values-stdin
```

### Google Slides

```bash
# プレゼンテーション情報を取得
gog-lite slides info --account you@gmail.com --presentation-id PRESENTATION_ID

# 全スライドのテキストを取得
gog-lite slides get --account you@gmail.com --presentation-id PRESENTATION_ID

# 特定スライドのテキストを取得
gog-lite slides get --account you@gmail.com --presentation-id PRESENTATION_ID \
  --page-id SLIDE_OBJECT_ID

# テキスト置換（--dry-run で確認してから）
gog-lite --dry-run slides write --account you@gmail.com --presentation-id PRESENTATION_ID \
  --find "{{NAME}}" --replace "Alice"

gog-lite slides write --account you@gmail.com --presentation-id PRESENTATION_ID \
  --find "{{NAME}}" --replace "Alice" --confirm-write
```

## 出力例

```bash
$ gog-lite gmail search --account you@gmail.com --query "is:unread" --max 3
{
  "messages": [
    {"id": "18c3a1b2c3d4e5f6", "thread_id": "18c3a1b2c3d4e5f6"},
    {"id": "18c3a1b2c3d4e5f5", "thread_id": "18c3a1b2c3d4e5f5"}
  ],
  "nextPageToken": ""
}

$ gog-lite gmail labels --account unknown@gmail.com
# → stderr:
{
  "error": "gmail options: read credentials: credentials.json not found at ...",
  "code": "gmail_error"
}
# exit code: 1

# Google API が HTTP 401 を返したとき（トークン未保存のときも同じ）
# → stderr: {"error":"...","code":"auth_required"}
# exit code: 2
```

## jq との組み合わせ

```bash
# 未読メールのIDだけ抽出
gog-lite gmail search --account you@gmail.com --query "is:unread" | jq -r '.messages[].id'

# 今日以降のイベントのタイトルと開始時刻
gog-lite calendar list --account you@gmail.com --from $(date -u +%Y-%m-%dT00:00:00Z) \
  | jq '.events[] | {summary, start}'

# ドキュメントのテキストだけ取り出す
gog-lite docs cat --account you@gmail.com --doc-id DOC_ID | jq -r .content
```

## ヘッドレス環境（Docker/CI）

credentials.json のマウントなしで環境変数だけで動かせる：

```bash
export GOG_LITE_CLIENT_ID=xxxx.apps.googleusercontent.com
export GOG_LITE_CLIENT_SECRET=GOCSPX-xxxx
export GOG_LITE_KEYRING_BACKEND=file
export GOG_LITE_KEYRING_PASSWORD=your-secure-password
```

| 変数 | 説明 |
|---|---|
| `GOG_LITE_CLIENT_ID` | OAuth クライアント ID |
| `GOG_LITE_CLIENT_SECRET` | OAuth クライアントシークレット |
| `GOG_LITE_KEYRING_BACKEND` | `file` でファイルバックエンドを強制。未設定かつ Linux でセッションバスが無いときもファイルバックエンド |
| `GOG_LITE_KEYRING_PASSWORD` | ファイルバックエンドの暗号化パスワード。空や空白だけは不可 |

`GOG_LITE_CLIENT_ID` と `GOG_LITE_CLIENT_SECRET` の両方が設定されている場合、credentials.json は不要。
macOS では上記2値を Keychain に保存しておけば、環境変数未設定でもフォールバックで参照できる。
ファイルバックエンドでは `GOG_LITE_KEYRING_PASSWORD` が必須。`GOG_LITE_KEYRING_BACKEND=file` を指定したときと、Linux でセッションバスが無くファイルバックエンドへ自動で切り替わるときの両方。未設定、空文字、空白だけは拒否する。端末へのパスワード入力は行わない。セッションバスは、`DBUS_SESSION_BUS_ADDRESS` か `/run/user/<uid>/bus` があるとあるものとして扱う。

## 監査ログ

書き込み系コマンドは、`--dry-run` を含めて、監査ログへ 1 行追記する。`--audit-log` を省略したときのパスは設定ディレクトリの `audit.log`。追記には keyring が必要で、失敗時の code は `audit_error`（終了コード 1）。ヘッドレスでは上の `GOG_LITE_KEYRING_BACKEND=file` と `GOG_LITE_KEYRING_PASSWORD` を設定する。Linux でセッションバスが無い場合もファイルバックエンドになる。`GOG_LITE_KEYRING_BACKEND` を省略しても `GOG_LITE_KEYRING_PASSWORD` は必要。

各行は JSON で、次のフィールドを持つ。

| フィールド | 意味 |
|---|---|
| `timestamp` | UTC の RFC3339 |
| `action` | 操作名 |
| `account` | アカウント。無いときは省略 |
| `target` | 対象 ID。無いときは省略 |
| `dry_run` | dry-run なら `true` |
| `prev_hash` | 直前の行の `hash`。先頭行では省略 |
| `hash_alg` | 新規行は `hmac-sha256`。それ以前の行では省略され、SHA-256 |
| `hash` | この行のハッシュ |

`hash` の材料は `timestamp|action|account|target|dry_run|prev_hash`。新規行はこの文字列の HMAC-SHA256 で、鍵は keyring の `audit-hmac-key`。同じ keyring に、ログパスごとの末尾ハッシュも残す。鍵も末尾ハッシュもログファイルには書かない。追記はファイルロックの中で読み書きする。ロックはそのプロセスが終了するまで有効で、5 秒以内に取れないときは `audit_error`（終了コード 1）になる。

`hash_alg` の無い既存行は SHA-256 のまま検証でき、新しい行はその末尾ハッシュを `prev_hash` にしてつながる。SHA-256 だけのファイルは、この版で最初に追記したときに末尾が keyring へ固定される。それより前にファイルだけを書き換えてチェーンを作り直した痕跡は、その初回追記では検知できない。初回追記のあと、末尾ハッシュがログの最後の行ともその 1 つ前とも一致しない追記は `audit_error` になる。末尾の更新が 1 行だけ遅れた状態は、追記の直後にプロセスが落ちたものとして次の追記で受け入れる。`audit-hmac-key` を消すと、既存の HMAC 行は検証できなくなる。

## 対応サービスと必要スコープ

`auth login --services` で選んだサービスごとに、ログイン時は次のスコープへ同意します。実行時の API クライアントは、操作に応じてその一部だけを要求します。

`docs export` は Drive API の `files.export` で既存のドキュメントを書き出します。`drive.file` は、このアプリが作成したファイルか、Google ピッカーで開いたファイルにしか届かないため、ドキュメント ID を指定するこのコマンドには足りません。読み取り専用の `drive.readonly` を使います。すべてのファイルを変更・削除できる `drive` スコープは要求しません。

| サービス | 有効化が必要なAPI | スコープ |
|---------|-----------------|---------|
| `gmail` | Gmail API | `gmail.readonly`, `gmail.compose`（操作に応じて最小権限） |
| `calendar` | Google Calendar API | `calendar.readonly` / `calendar`（操作に応じて最小権限） |
| `docs` | Docs API + Drive API | `documents.readonly` / `documents` / `drive.readonly`（操作に応じて最小権限） |
| `drive` | Google Drive API | `drive.readonly` |
| `sheets` | Google Sheets API | `spreadsheets.readonly` / `spreadsheets`（操作に応じて最小権限） |
| `slides` | Google Slides API | `presentations.readonly` / `presentations`（操作に応じて最小権限） |

## Contract Notes

- `gmail send` は Gmail draft を保存するためのコマンドです。送信操作を自動化対象にはしません。
- `--dry-run` は書き込み系操作の標準確認手段です。自動化では実行前に使う前提で設計します。
- 公開契約として安定して守るのは、stdout/stderr の JSON 形、終了コード、主要安全制御です。
- Google API の生レスポンス詳細より、ここに記載した CLI 契約を優先して利用してください。
- Google API の HTTP 429 は `Retry-After` を待って再試行する。秒数でも HTTP 日付でも、1 回の待ちは 30 秒まで。終了コードとエラー `code` は変わらない。

## ドキュメント

| ドキュメント | 対象 | 内容 |
|---|---|---|
| [docs/getting-started.md](docs/getting-started.md) | 初めて使う方 | インストール・Google Cloud 設定・初回認証・動作確認 |
| [AGENTS.md](AGENTS.md) | AI エージェント開発者 | Codex向け最小運用ルール（優先順位・禁止事項・テスト方針） |
| [docs/agents.md](docs/agents.md) | AI エージェント開発者 | コマンド仕様・エラーコード・ページネーション・stdin 活用（詳細版） |
| [docs/coding-rules.md](docs/coding-rules.md) | コントリビューター | 実装時の規約・出力契約・テストルール |
| [docs/review-checklist.md](docs/review-checklist.md) | レビュアー | バグ・回帰・セキュリティ観点のレビュー項目 |
| [docs/ci-playbook.md](docs/ci-playbook.md) | 開発者 | CI失敗時の切り分けとフレーク対処手順 |
| [docs/adr/README.md](docs/adr/README.md) | コントリビューター | アーキテクチャ判断（ADR） |

## CI セキュリティテスト方針

| ジョブ | トリガー | 内容 |
|---|---|---|
| `unit-fast` | PR / push | `go test ./...` + vet でユニットテストを高速実行 |
| `staticcheck` | PR / push | `staticcheck` v0.8.1。既定のチェックで、除外はない |
| `govulncheck` | PR / push / 毎日 02:00 UTC | 呼び出し経路上の脆弱性をスキャン |
| `integration-cli` | PR / push | 実バイナリを temp HOME で実行し、policy_denied（Sheets / Slides を含む）・approval_required・preflight スキーマなどの結合挙動を検証 |
| `security-nightly` | 毎日 02:00 UTC | 監査ログ改ざん検知・承認トークン再利用・stdin 上限・キーリング設定ミスの回帰テスト |

失敗時は JSON エラー本文を GitHub Actions Artifacts に保存します。

## ライセンス

MIT
