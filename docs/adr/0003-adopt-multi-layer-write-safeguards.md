# 0003: Adopt multi-layer safeguards for write operations

- Status: Accepted
- Date: 2026-02-27

## Context

書き込み系操作は誤操作・濫用・意図しない自動実行のリスクが高い。
単一チェックでは防げないケースがあるため、多層防御が必要。

## Historical Background

- PR #3 で OAuth step2 と stdin 入力境界を強化した。
- PR #4 と #5 で監査ログ、レートリミット、権限制御を拡張した。
- PR #7 で preflight / policy / approval-token を導入し、AI運用向け安全策を追加した。
- PR #26 で policy/approval の検証をさらに厳格化した。

## Decision

書き込み操作は以下の順でガードする。

- action/account に対する policy チェック
- 破壊的操作の confirm フラグ確認
- `--dry-run` の優先利用
- 必要アクションで approval-token を要求（使い捨て）
- 監査ログ（ハッシュチェーン）への記録
- action 単位のレートリミット
- stdin サイズ上限の強制

## Consequences

- 1つのガード漏れが即事故につながりにくくなる。
- 実装・テスト・運用の複雑性は増える。
- 安全性を優先するため、書き込み系 UX は意図的に厳しめになる。
