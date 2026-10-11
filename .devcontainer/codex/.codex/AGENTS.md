# Personal Codex Working Agreement

## Spec-Driven Development

ソフトウェア開発において、以下に該当する依頼では
`sdd-development` Skill を使用する。

- 新機能
- 既存仕様の変更
- API またはデータモデルの変更
- 複数ファイルに影響する変更
- 非自明なバグ修正
- アーキテクチャまたは依存関係の変更

標準フローは以下とする。

1. Requirements
2. Design
3. Tasks
4. Implementation
5. Verification
6. Reconciliation

Requirements、Design、Tasks の成果物が揃うまで、
プロダクションコードを変更してはならない。

各フェーズの終了時に成果物を提示して停止し、
ユーザーから明示的な承認を得てから次へ進む。

ユーザーが「実装して」と依頼した場合も、
承認済みの仕様がなければ Requirements から開始する。

既存のプロジェクト固有 AGENTS.md は、
リポジトリの規約、検証コマンド、ドキュメント更新規則として
引き続き遵守する。

プロジェクト固有指示と本ファイルが競合した場合は、
競合内容を報告し、推測で実装を開始しない。
