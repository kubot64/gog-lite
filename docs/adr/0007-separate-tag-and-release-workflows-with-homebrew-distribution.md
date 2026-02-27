# 0007: Separate tag/release workflows and distribute via Homebrew

- Status: Accepted
- Date: 2026-02-27

## Context

リリースの再現性と保守性を上げるには、タグ発行と配布処理を分離し、
失敗時の切り戻し点を明確にする必要がある。

## Historical Background

- PR #23 で GoReleaser と Homebrew 配布を導入した。
- PR #27/#28 で tag workflow と release workflow を分離し、連携トリガーを整備した。
- PR #29/#30 で release workflow の不整合を修正した。
- PR #40/#41 で配布系の安定化と Formula 更新運用を改善した。

## Decision

- タグ作成とリリース配布を別 workflow として運用する。
- Release は Tag 完了を契機に実行し、GoReleaser で成果物と Homebrew Formula を更新する。
- 配布系の変更は CI 検証と手順ドキュメントをセットで更新する。

## Consequences

- リリース失敗時の原因切り分けがしやすくなる。
- workflow 間依存が増えるため、設定ミス時の不具合パターンが増える。
- リリース運用の可視化には docs/ci-playbook の継続更新が必要。
