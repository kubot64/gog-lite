# 0001: Use ADR for architecture decisions

- Status: Accepted
- Date: 2026-02-27

## Context

これまでアーキテクチャ情報を `docs/architecture.md` に集約していたため、
時系列の意思決定理由と変更履歴が追いにくかった。

## Historical Background

- 初期は利用者向けの理解コストを下げるため、単一ドキュメント
  `docs/architecture.md` に全体像を集約していた。
- 機能追加（policy / approval-token / audit-log / ratelimit）に伴い、
  仕様変更の背景やトレードオフが単一ドキュメントでは埋もれやすくなった。
- PR レビューでも「現状仕様」は追える一方、「なぜそうなったか」の参照先が不足し、
  判断の再検討コストが増えていた。

## Decision

アーキテクチャ判断は Markdown の単一解説ではなく ADR で管理する。

- 今後の設計判断は `docs/adr/` に `NNNN-title.md` 形式で追加する。
- 既存判断の更新は、元 ADR を直接書き換えず、新しい ADR を追加して supersede する。
- `docs/architecture.md` は正本としては運用しない。

## Consequences

- 判断理由の追跡性が上がる。
- レビュー時に「なぜその設計か」を PR と ADR で紐づけやすくなる。
- 既存の `docs/architecture.md` 参照は `docs/adr/README.md` へ順次移行する必要がある。
