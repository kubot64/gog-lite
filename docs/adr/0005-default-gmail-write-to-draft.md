# 0005: Default Gmail write path to draft instead of immediate send

- Status: Accepted
- Date: 2026-02-27

## Context

AI エージェント経由のメール操作では、即時送信は誤送信時の影響が大きい。
まず下書きとして保存し、人手確認を経る方が安全。

## Historical Background

- PR #22 で Gmail 書き込みのデフォルト挙動を「送信」から「下書き保存」へ変更した。
- 変更に合わせて README と agents ドキュメントを更新し、運用前提を明示した。

## Decision

- Gmail の書き込み系コマンドはデフォルトで draft を作成する。
- 直接送信は標準パスにしない。
- 利用者には「下書き確認後に送信」を推奨フローとして案内する。

## Consequences

- 誤送信リスクを大きく下げられる。
- 即時送信が必要なユースケースでは手順が1段増える。
- 仕様理解のズレを防ぐため、ドキュメント整合性が重要になる。
