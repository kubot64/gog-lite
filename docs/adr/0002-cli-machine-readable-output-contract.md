# 0002: Establish machine-readable CLI output contract

- Status: Accepted
- Date: 2026-02-27

## Context

AI エージェント運用では、CLI 出力が安定して機械解析できることが必須。
表示向けフォーマット（色・表・自由文）が混在すると自動化が壊れやすい。

## Historical Background

- PR #1 で CLI の挙動とエラーハンドリングを仕様寄りに整理した。
- PR #43 で「stdout/stderr/終了コード」の契約を再度明文化し、実装を厳密化した。

## Decision

- stdout は常に JSON オブジェクトを返す。
- stderr は常に `{"error":"...","code":"..."}` 形式の JSON を返す。
- 終了コードは `0=成功 / 1=エラー / 2=認証エラー / 3=未発見 / 4=権限なし` に固定する。
- 色付き出力・表形式・TSV などの非機械可読形式は採用しない。

## Consequences

- 自動化ワークフローでパース失敗が起きにくくなる。
- 人間向けの見やすさより、契約互換性の維持を優先する必要がある。
- 出力仕様変更時は後方互換性評価とテスト更新が必須になる。
