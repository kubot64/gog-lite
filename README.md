# gog-lite

AIエージェントが Gmail / Google Calendar / Google Docs を操作するためのシンプルな CLI。

[gogcli](https://github.com/steipete/gogcli) の多機能さをAIエージェント向けに絞り込んだ派生版。

## Public Contract

- **stdout は常に JSON** — `--help` / `--version` を含め、色・表・TSV は出力しない
- **stderr は常に JSON エラー** — `{"error": "...", "code": "..."}` を返し、stdout と混在しない
- **終了コードは固定** — `0=成功 / 1=エラー / 2=認証エラー / 3=未発見 / 4=権限なし`
- **破壊的操作には安全制御がある** — `confirm` フラグと、必要に応じて `approval-token` を要求する
- **Sheets のセル値は既定で RAW** — `=` で始まる文字列を数式にしない。数式にするときだけ `--value-input USER_ENTERED`。`sheets update` は `--confirm-write` と approval-token が必要
- **`--dry-run` は書き込み前の標準確認手段** — 実行前に API 呼び出しなしで内容確認できる
- **`gmail send` は下書き保存契約** — 即時送信ではなく Gmail draft として保存する

## Safety Controls

- `--dry-run` — 書き込み前の事前確認
- `--audit-log` — 書き込み操作の JSONL 監査ログ
- `--allowed-output-dir` — ファイル出力先ディレクトリ制限
- `--confirm-*` — 破壊的操作の明示確認
- `--approval-token` — 高リスク操作の追加承認

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

`auth emergency-revoke` は、ローカルのトークンを消す前に refresh token を `https://oauth2.googleapis.com/revoke` へ POST する。Google が HTTP 200 を返したときだけ、ローカルのトークンを削除し、policy の `blocked_accounts` にアカウントを追加する。

```json
{"revoked": true, "blocked": true, "email": "you@gmail.com"}
```

失効に失敗したときはローカル削除もブロックもせず、stderr に `code` が `remote_revoke_failed` の JSON を出す。`error` は「リモート失効に失敗した」で始まる。通信失敗や 200 以外の応答は終了コード 1。保存された refresh token が無いときは終了コード 3。

`auth remove` はローカルのトークンを削除するだけで、Google 側は失効しない。

### Gmail

```bash
# 未読メールを検索
gog-lite gmail search --account you@gmail.com --query "is:unread" --max 10

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

# 今週のイベントを取得
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

# 内容の書き込み（--replace で全置換）
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
| `GOG_LITE_KEYRING_BACKEND` | `file` でファイルバックエンドを強制 |
| `GOG_LITE_KEYRING_PASSWORD` | ファイルバックエンドの暗号化パスワード |

`GOG_LITE_CLIENT_ID` と `GOG_LITE_CLIENT_SECRET` の両方が設定されている場合、credentials.json は不要。
macOS では上記2値を Keychain に保存しておけば、環境変数未設定でもフォールバックで参照できる。
`GOG_LITE_KEYRING_BACKEND=file` の場合、`GOG_LITE_KEYRING_PASSWORD` は必須。

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
| `integration-cli` | PR / push | 実バイナリを temp HOME で実行し、policy_denied・approval_required・preflight スキーマなどの結合挙動を検証 |
| `security-nightly` | 毎日 02:00 UTC | 監査ログ改ざん検知・承認トークン再利用・stdin 上限・キーリング設定ミスの回帰テスト |

失敗時は JSON エラー本文を GitHub Actions Artifacts に保存します。

## ライセンス

MIT
