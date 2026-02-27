# 0004: Standardize auth flow and credential handling for headless use

- Status: Accepted
- Date: 2026-02-27

## Context

CLI はローカル/CI/ヘッドレス環境で使われるため、認証と資格情報の取り扱いを
環境依存にしない設計が必要。

## Historical Background

- PR #3 でヘッドレス運用前提の OAuth step2 の安全性を強化した。
- 初期実装で `GOG_LITE_CLIENT_ID` / `GOG_LITE_CLIENT_SECRET` を導入し、
  `credentials.json` なしでも起動できるようにした。
- PR #24 で macOS Keychain fallback を追加し、資格情報の保管経路を拡張した。

## Decision

- OAuth は 2-step（URL生成 → コード交換）を標準フローとする。
- OAuth クライアント情報は `credentials.json` または環境変数で受け取る。
- トークン・資格情報は OS セキュアストアを優先し、必要時に fallback を使う。
- ヘッドレス環境でも同一コマンド契約で動作することを維持する。

## Consequences

- CI/自動化/ローカル運用で認証手順を共通化できる。
- 資格情報の保存先が複数になり、障害切り分け時は優先順の理解が必要。
- ドキュメントで環境別セットアップ手順を継続的に同期する必要がある。
