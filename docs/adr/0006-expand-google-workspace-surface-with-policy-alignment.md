# 0006: Expand Google Workspace surface with policy-aligned controls

- Status: Accepted
- Date: 2026-02-27

## Context

Gmail/Calendar/Docs だけでは自動化ニーズを満たしきれず、Sheets/Slides 連携が必要になった。
一方で対象 API の拡大は権限面のリスク増加を伴う。

## Historical Background

- PR #25 で Sheets/Slides サポートを追加した。
- 既存の policy/approval モデル（PR #5/#7/#26）に新アクションを統合した。

## Decision

- サポート対象に Google Sheets / Google Slides を追加する。
- 追加アクションも既存の `allowed_actions`・`require_approval_actions`・監査対象に含める。
- 新規サービス追加時は「機能追加」と「制御面の統合」を同時に行う。

## Consequences

- 自動化可能な業務範囲が広がる。
- action 管理とテスト対象が増え、メンテナンスコストが上がる。
- 権限の最小化を維持するため、ポリシー更新の運用がより重要になる。
