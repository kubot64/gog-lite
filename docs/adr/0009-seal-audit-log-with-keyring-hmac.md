# 0009: Seal audit logs with a keyring HMAC and a stored tail

- Status: Accepted
- Date: 2026-10-11

## Context

監査ログは SHA-256 のハッシュチェーンだった。追記は直前ハッシュを読んでからファイルへ足すが、その間にロックが無い。並行して追記すると `prev_hash` が分岐する。ハッシュの材料に秘密が無いので、ログファイルを書き替えられる主体はチェーンを作り直せる。

ADR 0003 は書き込み操作の記録先としてハッシュチェーンを採用している。この ADR は、そのチェーンの作り方だけを置き換える。

## Decision

- 直前ハッシュの読み取りと追記は、対象ログのファイルロックの中で行う。
- 新しい行の `hash` は HMAC-SHA256 とする。鍵は keyring の `audit-hmac-key` に置き、ログファイルには書かない。行の `hash_alg` は `hmac-sha256`。
- ログパスごとの末尾ハッシュも keyring に残す。追記前に、ファイル上のチェーンとこの末尾が一致することを確認する。
- `hash_alg` の無い既存行は、従来の SHA-256（材料は `timestamp|action|account|target|dry_run|prev_hash`）として検証する。新しい行は、その末尾を `prev_hash` にしてつなぐ。
- 末尾の更新が 1 行分だけ遅れた状態は、追記と末尾更新の間でプロセスが落ちたものとして受け入れる。それ以外の不一致では追記しない。

## Consequences

- ログファイルだけを書き換えても、keyring を持たない主体はチェーンを作り直せない。
- 追記には keyring が必要になる。ファイルバックエンドでは `GOG_LITE_KEYRING_PASSWORD` が要る。Linux で D-Bus が無いときもファイルバックエンドになり、パスワードが空だと追記できない。失敗時の code は従来どおり `audit_error`。
- この版で最初に追記するより前の、SHA-256 だけのファイルは、まだ末尾が keyring に無い。その間の作り直しは検知できない。
- `audit-hmac-key` を消すと、既存の HMAC 行は検証できなくなる。
