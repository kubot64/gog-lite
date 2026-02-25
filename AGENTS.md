# gog-lite — Codex Agent Guide

このファイルは **Codex 向けの最小運用ルール** です。詳細仕様は `docs/` 配下を参照。

## 優先順位

1. `AGENTS.md`（本ファイル）
2. `docs/*.md`（詳細ルール・手順）
3. `README.md`（利用者向け説明）

矛盾がある場合は上から優先して判断する。

## 必須ルール

- stdout は常に JSON。色・表・TSV を出力しない。
- エラーは stderr に `{"error":"...","code":"..."}` を出力する。
- 終了コードは `0=成功 / 1=エラー / 2=認証エラー / 3=未発見 / 4=権限なし` を守る。
- 各コマンドで `--account` は必須（グローバルではない）。
- 書き込み系の検証では `--dry-run` を優先して使う。
- 破壊的操作（削除・置換・find-replace）には confirm フラグ・approval-token 要件を維持する。

## コーディング時の注意

- 既存スタイルに合わせ、最小差分で修正する。
- 無関係なリファクタやファイル移動を同PRに混ぜない。
- セキュリティ機能（policy / approval-token / audit-log / ratelimit）の後方互換を壊さない。
- 秘密情報（credentials、token、パスワード）をコード・ログ・PR本文に含めない。
- CI失敗系テストでは共有 `/tmp` 固定ファイルを避け、`mktemp` と非空チェックを使う。

## テスト・レビュー方針

- 変更箇所に対応するテストを最小で実行し、必要なら `go test ./...` まで拡張する。
- レビューはバグ・回帰・テスト不足を優先し、見た目変更は後回しにする。
- PRには「目的 / 変更点 / テスト結果 / 影響範囲」を明記する。

## 詳細ドキュメント

- 仕様リファレンス: [`docs/agents.md`](docs/agents.md)
- コーディング規約: [`docs/coding-rules.md`](docs/coding-rules.md)
- レビュー観点: [`docs/review-checklist.md`](docs/review-checklist.md)
- CI運用手順: [`docs/ci-playbook.md`](docs/ci-playbook.md)
- アーキテクチャ: [`docs/architecture.md`](docs/architecture.md)
- 初期セットアップ: [`docs/getting-started.md`](docs/getting-started.md)
