# Getting Started

このガイドでは **gog-lite を初めてセットアップして最初のコマンドを実行するまで** を順を追って説明します。

---

## 前提条件

- Go 1.24 以上がインストールされていること
- Google アカウント（Gmail）を持っていること
- ターミナルの基本操作ができること

---

## 1. インストール

```bash
git clone https://github.com/kubot64/gog-lite
cd gog-lite
go build -o ~/bin/gog-lite ./cmd/gog-lite/
```

`~/bin` にパスが通っていない場合は以下を `~/.zshrc` または `~/.bashrc` に追加してください。

```bash
export PATH="$HOME/bin:$PATH"
```

動作確認：

```bash
gog-lite --version
# gog-lite 0.1.0
```

---

## 2. Google Cloud でアプリを登録する

gog-lite は Google の OAuth を使って Gmail / Calendar / Docs にアクセスします。
最初に一度だけ Google Cloud Console での設定が必要です。

### 2-1. プロジェクトを作成する

1. [Google Cloud Console](https://console.cloud.google.com/) を開く
2. 画面上部のプロジェクト選択 → **「新しいプロジェクト」**
3. 任意の名前をつけて作成

### 2-2. 使用する API を有効化する

左メニュー → **「APIとサービス」→「ライブラリ」** から以下を検索して有効化：

| 使いたい機能 | 有効化する API |
|---|---|
| メール操作 | Gmail API |
| カレンダー操作 | Google Calendar API |
| ドキュメント操作 | Google Docs API・Google Drive API |

### 2-3. OAuth クライアントを作成する

1. 左メニュー → **「APIとサービス」→「認証情報」**
2. **「認証情報を作成」→「OAuth クライアント ID」**
3. アプリケーションの種類：**「デスクトップアプリ」** を選択
4. 任意の名前をつけて **「作成」**
5. ダイアログが開いたら **「JSON をダウンロード」**

### 2-4. OAuth 同意画面でテストユーザーを追加する（External の場合）

1. 左メニュー → **「Google Auth Platform」→「Audience」**
2. 画面下部の **「Test users」** で **「Add users」**
3. 利用する Gmail（例: `you@gmail.com`）を追加して保存

> `このアプリは Google で確認されていません` はテスト中アプリでは通常表示されます。  
> 自分で作成したアプリであれば「続行」で問題ありません。

### 2-5. 認証情報ファイルを配置する

```bash
# macOS
mkdir -p "$HOME/Library/Application Support/gog-lite"
cp ~/Downloads/client_secret_*.json "$HOME/Library/Application Support/gog-lite/credentials.json"

# Linux
mkdir -p ~/.config/gog-lite
cp ~/Downloads/client_secret_*.json ~/.config/gog-lite/credentials.json
```

---

## 3. Google アカウントを認証する

gog-lite はブラウザを自動起動しない **2 ステップの認証フロー** を採用しています。

### ステップ 1：認証 URL を取得する

```bash
gog-lite auth login --account you@gmail.com --services gmail,calendar,docs
```

以下のような JSON が出力されます：

```json
{
  "auth_url": "https://accounts.google.com/o/oauth2/auth?...",
  "next_step": "Open auth_url in a browser, then run this command again with --auth-url <redirect_url>"
}
```

### ステップ 2：ブラウザで認可してトークンを保存する

1. `auth_url` の値をブラウザで開く
2. Google アカウントでログインして「許可」
3. リダイレクト先の URL（`http://127.0.0.1:...?code=...` のような URL）をコピー
   - ページが読み込めなくてもOK。URL だけコピーすれば大丈夫です
4. コピーした URL を `--auth-url` に渡す：

```bash
gog-lite auth login --account you@gmail.com --services gmail,calendar,docs \
  --auth-url "http://127.0.0.1:PORT/oauth2/callback?code=..."
```

成功すると：

```json
{
  "stored": true,
  "email": "you@gmail.com",
  "services": ["gmail", "calendar", "docs"]
}
```

---

## 4. 動作確認

認証が完了したら準備 OK かチェックします：

```bash
gog-lite auth preflight --account you@gmail.com
```

```json
{
  "ready": true,
  "email": "you@gmail.com",
  "checks": [
    {"name": "credentials", "ok": true},
    {"name": "keyring",     "ok": true},
    {"name": "token",       "ok": true}
  ]
}
```

`"ready": true` であれば完了です。

---

## 5. 最初のコマンドを試す

### 未読メールを確認する

```bash
gog-lite gmail search --account you@gmail.com --query "is:unread" --max 5
```

### 今日のカレンダーを確認する

```bash
gog-lite calendar list --account you@gmail.com \
  --from $(date -u +%Y-%m-%dT00:00:00Z) \
  --to $(date -u +%Y-%m-%dT23:59:59Z)
```

---

## トラブルシューティング

### `credentials.json not found` と出る

`gog-lite` は `os.UserConfigDir()` 配下の `gog-lite/credentials.json` を参照します。  
macOS と Linux でパスが異なるため、両方確認してください。

```bash
# macOS
ls "$HOME/Library/Application Support/gog-lite/"

# Linux
ls ~/.config/gog-lite/
```

### `ready: false` で `token: false` になる

ステップ 3 の認証が完了していません。`auth login` を最初からやり直してください。

### `エラー 403: access_denied` が出る

OAuth 同意画面の **Test users** に対象 Gmail が追加されていない可能性があります。  
`Google Auth Platform -> Audience -> Test users` から追加して再実行してください。

### `keyring_error` が出る（Linux / ヘッドレス環境）

OS のキーリングが使えない環境では、ファイルバックエンドを使います：

```bash
export GOG_LITE_KEYRING_BACKEND=file
export GOG_LITE_KEYRING_PASSWORD=任意のパスワード
```

これを `~/.zshrc` / `~/.bashrc` に追加しておくと毎回設定不要になります。

---

## 次のステップ

- より詳しいコマンド一覧 → [README](../README.md)
- AI エージェントとして使う → [AGENTS.md](../AGENTS.md)
- 内部構造を理解する → [docs/architecture.md](architecture.md)
