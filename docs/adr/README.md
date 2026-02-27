# Architecture Decision Records (ADR)

このディレクトリは、アーキテクチャ判断を時系列で記録するための ADR 置き場です。

## 運用ルール

- 形式: `NNNN-title.md`（例: `0001-adopt-adr-for-architecture-decisions.md`）
- 追加のみ: 既存 ADR は原則「追記しない」。方針変更は新しい ADR で置き換える。
- 参照: PR で設計判断がある場合は、該当 ADR を本文にリンクする。
- 状態: `Proposed` / `Accepted` / `Superseded` / `Deprecated` を持つ。

## ADR 一覧

- [0001: Use ADR for architecture decisions](0001-adopt-adr-for-architecture-decisions.md)
- [0002: Establish machine-readable CLI output contract](0002-cli-machine-readable-output-contract.md)
- [0003: Adopt multi-layer safeguards for write operations](0003-adopt-multi-layer-write-safeguards.md)
- [0004: Standardize auth flow and credential handling for headless use](0004-standardize-auth-and-credential-handling.md)
- [0005: Default Gmail write path to draft instead of immediate send](0005-default-gmail-write-to-draft.md)
- [0006: Expand Google Workspace surface with policy-aligned controls](0006-expand-google-workspace-surface-with-policy-alignment.md)
- [0007: Separate tag/release workflows and distribute via Homebrew](0007-separate-tag-and-release-workflows-with-homebrew-distribution.md)
- [0008: Improve CI reliability and supply-chain security checks](0008-improve-ci-reliability-and-supply-chain-security.md)
