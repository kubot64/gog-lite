# gog-lite

AIエージェントが Gmail / Google Calendar / Google Docs を操作するためのシンプルな CLI。

[gogcli](https://github.com/steipete/gogcli) の多機能さをAIエージェント向けに絞り込んだ派生版。

## 特徴

- **JSON専用出力** — stdout は常にJSON。色・表・TSVなし
- **ヘッドレス認証** — ブラウザ自動起動なし。URLを出力して2ステップで認証完了
- **予測可能な終了コード** — `0`=成功 / `1`=エラー / `2`=認証エラー / `3`=未発見 / `4`=権限なし
- **エラーはstderrにJSON** — `{"error": "...", "code": "..."}` 形式。stdoutと混在しない
- **stdin対応** — `--body-stdin` / `--content-stdin` でパイプ渡し可能
- **dry-run** — 書き込み系コマンドを `--dry-run`（`-n`）で確認できる

## インストール

```bash
git clone https://github.com/morikubo-takashi/gog-lite
cd gog-lite
go build -o ~/bin/gog-lite ./cmd/gog-lite/
```

## セットアップ

### 1. OAuth クライアント認証情報

[Google Cloud Console](https://console.cloud.google.com/) で以下を行う：

1. プロジェクトを作成（または既存を使用）
2. 必要なAPIを有効化：Gmail API / Google Calendar API / Google Docs API / Google Drive API
3. 「認証情報」→「OAuthクライアントID」を作成（種類：**デスクトップアプリ**）
4. JSONをダウンロードして配置：

```bash
mkdir -p ~/.config/gog-lite
cp ~/Downloads/client_secret_*.json ~/.config/gog-lite/credentials.json
```

### 2. アカウント認証（2ステップ）

```bash
# ステップ1: 認証URLを取得
gog-lite auth login --account you@gmail.com --services gmail,calendar,docs
# → {"auth_url": "https://accounts.google.com/...", "next_step": "..."}
```

ブラウザで `auth_url` を開いて認証する。リダイレクト先のURL（読み込めなくてOK）をコピーして：

```bash
# ステップ2: リダイレクトURLを渡してトークンを保存
gog-lite auth login --account you@gmail.com --services gmail,calendar,docs \
  --auth-url "http://127.0.0.1:PORT/oauth2/callback?code=..."
# → {"stored": true, "email": "you@gmail.com", "services": [...]}
```

## 使い方

### 認証管理

```bash
gog-lite auth list                        # 認証済みアカウント一覧
gog-lite auth remove --account EMAIL      # アカウントのトークンを削除
```

### Gmail

```bash
# 未読メールを検索
gog-lite gmail search --account you@gmail.com --query "is:unread" --max 10

# メール本文を取得
gog-lite gmail get --account you@gmail.com --message-id MESSAGE_ID

# メールを送信
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
gog-lite calendar delete --account you@gmail.com --event-id EVENT_ID
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
gog-lite docs write --account you@gmail.com --doc-id DOC_ID --content "新しい内容" --replace

# エクスポート
gog-lite docs export --account you@gmail.com --doc-id DOC_ID --format pdf --output ~/Downloads/doc.pdf

# テキスト置換
gog-lite docs find-replace --account you@gmail.com --doc-id DOC_ID --find "旧文言" --replace "新文言"
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

macOS Keychain が使えない環境では、ファイルバックエンドを使用：

```bash
export GOG_LITE_KEYRING_BACKEND=file
export GOG_LITE_KEYRING_PASSWORD=your-secure-password
```

## 対応サービスと必要スコープ

| サービス | 有効化が必要なAPI | スコープ |
|---------|-----------------|---------|
| `gmail` | Gmail API | `gmail.modify`, `gmail.settings.basic` |
| `calendar` | Google Calendar API | `calendar` |
| `docs` | Docs API + Drive API | `documents`, `drive` |
| `drive` | Google Drive API | `drive` |

## AIエージェント向け詳細

コマンド仕様・エラーコード・ページネーション・stdin活用など詳細は [`AGENTS.md`](AGENTS.md) を参照。

## ライセンス

MIT
