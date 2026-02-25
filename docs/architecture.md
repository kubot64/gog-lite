# Architecture

gog-lite は AI エージェントが安全に Google API を操作するための CLI ツールです。
このドキュメントでは内部構造・データフロー・セキュリティ設計を説明します。

---

## パッケージ構成

```mermaid
graph TD
    main["cmd/gog-lite/main.go<br/><i>エントリポイント</i>"]

    subgraph internal
        cmd["internal/cmd<br/><i>コマンド実装</i><br/>gmail / calendar / docs / auth<br/>policy / approval / audit / ratelimit"]
        config["internal/config<br/><i>設定読み書き</i><br/>credentials.json / policy.json"]
        googleapi["internal/googleapi<br/><i>Google API クライアント生成</i><br/>Gmail / Calendar / Docs / Drive"]
        googleauth["internal/googleauth<br/><i>OAuth ヘッドレスフロー</i><br/>Step1(URL生成) / Step2(コード交換)"]
        secrets["internal/secrets<br/><i>トークン永続化</i><br/>OS キーリング / ファイルバックエンド"]
        output["internal/output<br/><i>出力ヘルパー</i><br/>JSON stdout / エラー stderr"]
    end

    main --> cmd
    cmd --> config
    cmd --> googleapi
    cmd --> secrets
    cmd --> output
    googleapi --> googleauth
    googleapi --> secrets
    googleauth --> config
    secrets --> config
```

---

## 書き込みコマンドのリクエストフロー

読み取りコマンド（`gmail search` など）はシンプルに API を呼ぶだけですが、
書き込みコマンドは以下の多段チェックを通過します。

```mermaid
flowchart TD
    A([CLI 呼び出し]) --> B{policy チェック\nenforceActionPolicy}
    B -- 拒否 --> E1([stderr: policy_denied\nexit 4])
    B -- 通過 --> C{confirm フラグ\n確認}
    C -- 未指定 --> E2([stderr: delete_requires_confirmation\nexit 1])
    C -- あり --> D{dry-run?}
    D -- Yes --> DRY([stdout: dry_run=true JSON\n監査ログに dry_run=true 記録])
    D -- No --> F{approval token\n必要?}
    F -- 不要 --> H
    F -- 必要 --> G{consumeApprovalToken\nトークン検証・消費}
    G -- 無効/期限切れ/再利用 --> E3([stderr: approval_required\nexit 4])
    G -- 成功 --> H[Google API 呼び出し]
    H -- 失敗 --> E4([stderr: API エラー JSON\nexit 1 or 2])
    H -- 成功 --> I[監査ログ記録\nappendAuditLog]
    I --> J([stdout: 結果 JSON\nexit 0])
```

> **confirm フラグが必要なコマンド**：`calendar delete` → `--confirm-delete`、`docs write --replace` → `--confirm-replace`、`docs find-replace` → `--confirm-find-replace`

---

## セキュリティレイヤー

### 1. ポリシー制御（`internal/cmd/policy.go`）

`~/.config/gog-lite/policy.json` でアクションとアカウントを制限します。

```json
{
  "allowed_actions": ["gmail.search", "calendar.get", "gmail.send"],
  "blocked_accounts": ["untrusted@example.com"],
  "require_approval_actions": ["calendar.delete", "docs.write.replace"]
}
```

- `allowed_actions` が空の場合はすべてのアクションを許可（デフォルト動作）
- `require_approval_actions` が空の場合はデフォルトセット（`calendar.delete`、`docs.write.replace`、`docs.find_replace`）を使用

### 2. 承認トークン（`internal/cmd/approval.go`）

危険な操作には使い捨ての承認トークンが必要です。

```mermaid
sequenceDiagram
    participant Agent as AI エージェント
    participant CLI as gog-lite
    participant FS as ファイルシステム

    Agent->>CLI: auth approval-token --action calendar.delete --ttl 10m
    CLI->>FS: approvals/<token>.json を作成
    CLI-->>Agent: {"token": "abc123", "expires_at": "..."}

    Agent->>CLI: calendar delete --approval-token abc123
    CLI->>FS: approvals/abc123.json を読み込み
    Note over CLI,FS: 期限・アカウント・アクションを検証
    CLI->>FS: used=true に更新（以降は再利用不可）
    CLI->>CLI: Google API 呼び出し
    CLI-->>Agent: {"deleted": true}

    Agent->>CLI: calendar delete --approval-token abc123（再利用）
    CLI->>FS: approvals/abc123.json を読み込み
    CLI-->>Agent: stderr: approval_required（already used）
```

### 3. 監査ログのハッシュチェーン（`internal/cmd/audit.go`）

書き込み操作はすべて JSONL 形式でログに記録され、各エントリは前のエントリの SHA-256 ハッシュを持ちます。

```mermaid
block-beta
  columns 3
  E1["エントリ 1\naction: gmail.send\nhash: a1b2c3\nprev_hash: (空)"]
  arrow1["→"]
  E2["エントリ 2\naction: calendar.create\nhash: d4e5f6\nprev_hash: a1b2c3"]
  arrow2["→"]
  E3["エントリ 3\naction: docs.write\nhash: 789abc\nprev_hash: d4e5f6"]
```

エントリを書き換えると `hash` の再計算値が変わり、次エントリの `prev_hash` と一致しなくなるため改ざんを検出できます。

### 4. レートリミット（`internal/cmd/ratelimit.go`）

アクションごとにタイムスタンプをスライディングウィンドウで管理します。

| アクション | 上限 | ウィンドウ |
|---|---|---|
| `gmail.search` | 120 回 | 1 分 |
| `gmail.send` | 20 回 | 1 分 |
| `calendar.list` | 120 回 | 1 分 |
| `docs.cat` | 120 回 | 1 分 |

### 5. stdin 上限（`internal/cmd/stdin.go`）

`--body-stdin` / `--content-stdin` の入力は 10 MB を超えるとエラーになります。

---

## OAuth 認証フロー（ヘッドレス 2 ステップ）

ブラウザを自動起動せず、URL を出力して手動で認可します。

```mermaid
sequenceDiagram
    participant Agent as AI エージェント
    participant CLI as gog-lite
    participant GCP as Google

    Note over Agent,GCP: Step 1 — 認証 URL の取得
    Agent->>CLI: auth login --account you@gmail.com --services gmail,calendar
    CLI->>CLI: credentials.json を読み込み
    CLI-->>Agent: {"auth_url": "https://accounts.google.com/...", "next_step": "..."}
    Agent-->>Agent: ユーザーがブラウザで auth_url を開く

    Note over Agent,GCP: Step 2 — コード交換
    Agent->>CLI: auth login --auth-url "http://127.0.0.1:PORT/callback?code=..."
    CLI->>GCP: コードをリフレッシュトークンに交換
    GCP-->>CLI: refresh_token
    CLI->>CLI: OS キーリングにトークンを保存
    CLI-->>Agent: {"stored": true, "email": "you@gmail.com"}
```

---

## 出力設計

AI エージェントが確実に結果をパースできるよう、stdout と stderr を厳密に分離します。

```
stdout  → 正常結果（常に JSON オブジェクト）
stderr  → エラー（常に {"error": "...", "code": "..."} 形式の JSON）
```

| 終了コード | 意味 |
|---|---|
| `0` | 成功 |
| `1` | エラー（一般） |
| `2` | 認証エラー（トークンなし・期限切れ） |
| `3` | 未発見（404 相当） |
| `4` | 権限なし（policy_denied / approval_required） |

---

## 設定ファイルの配置

すべての実行時データは XDG に従い `~/.config/gog-lite/` 以下に置きます。

```
~/.config/gog-lite/
├── credentials.json     # OAuth クライアント ID/シークレット
├── policy.json          # アクション制限・アカウントブロック
├── audit.log            # 書き込み操作の監査ログ（JSONL）
├── approvals/           # 承認トークンファイル（使い捨て）
│   └── <token>.json
├── ratelimit/           # レートリミット状態
│   ├── gmail.search.json
│   └── ...
└── keyring/             # OS キーリング or ファイルバックエンド
    └── <encrypted tokens>
```

> `credentials.json` は Google Cloud Console からダウンロードしたものをそのまま配置します。
> ヘッドレス環境では `GOG_LITE_CLIENT_ID` / `GOG_LITE_CLIENT_SECRET` 環境変数で代替できます。
