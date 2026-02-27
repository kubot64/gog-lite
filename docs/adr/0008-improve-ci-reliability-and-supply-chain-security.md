# 0008: Improve CI reliability and supply-chain security checks

- Status: Accepted
- Date: 2026-02-27

## Context

CI が不安定だと開発速度と品質保証の両方が落ちる。
同時に、GitHub Actions 依存の供給網リスクへ継続対応が必要。

## Historical Background

- PR #18/#38/#42 で flaky テスト対策と決定的な検証に寄せる改善を実施した。
- PR #31 で workflow 定義のローカル/CI 検証を追加した。
- PR #32 で action pinning と ref 検証を導入し、供給網リスク対策を強化した。

## Decision

- CI は「再現可能な失敗」を優先し、flaky 要因を継続排除する。
- workflow ファイルは専用スクリプトで静的検証する。
- GitHub Actions 参照は pinning と ref 検証を必須とする。
- 定期実行はノイズを抑え、信号として機能する頻度に調整する。

## Consequences

- CI 信頼性が上がり、レビューとリリース判断がしやすくなる。
- ルール強化でメンテナンス作業（更新・追従）は増える。
- 依存更新時は security check を通すための追加作業が必要になる。
