# SDD Review Checklists

各フェーズを承認済みに変更する前に、対応するチェックリストを使用する。該当しない項目には理由を記録する。

## Discovery review

- リポジトリの `AGENTS.md`、README、開発ガイドを確認した
- 既存の仕様、関連コード、テスト、API 定義を確認した
- 検証コマンドを特定した
- 既存の未コミット変更を確認した
- Feature Spec、Bugfix Spec、Spec-exempt のいずれかに分類した
- 仕様成果物の配置場所を決定した

## Requirements review

- Goals と Non-goals が分離されている
- 各要求に一意の Requirement ID がある
- 各要求に検証可能な Acceptance Criteria がある
- Acceptance Criteria が観測可能な振る舞いを記述している
- 正常系、異常系、境界条件が定義されている
- 曖昧語に具体的な条件または測定基準がある
- 非機能、互換性、セキュリティへの影響を確認した
- 重要な未解決事項が残っていない
- ユーザーの承認を得た

## Bug analysis review

- Current behavior を再現または証拠で確認した
- Expected behavior が観測可能な形で定義されている
- Unchanged behavior が定義されている
- Confirmed evidence、Hypothesis、Unknown を区別した
- 根本原因が確認済みか未確認か明示した
- 回帰リスクを整理した
- 修正を検証する Acceptance Criteria がある
- ユーザーの承認を得た

## Design review

- すべての Requirement ID が設計へ対応付けられている
- コンポーネントの責務とデータフローが明確である
- API とデータモデルへの影響が記載されている
- エラー処理が定義されている
- セキュリティ、互換性、移行方法を検討した
- ログ、メトリクス、トレースの必要性を検討した
- Requirement ごとのテスト戦略がある
- 代替案と不採用理由が記録されている
- リスクと軽減策が記録されている
- 重要な未解決事項が残っていない
- ユーザーの承認を得た

## Tasks review

- すべての Requirement ID が1つ以上の Task に対応している
- 各 Task に Requirement、Design、依存関係、対象ファイルがある
- 各 Task に観測可能な完了条件がある
- 可能な場合は失敗するテストを先に追加する
- 回帰テストとドキュメント更新が含まれている
- タスクが独立して実行・検証できる大きさである
- Requirement → Design → Task → Test を追跡できる
- ユーザーの承認を得た

## Implementation review

- 実行中の Task を `[~]` に更新した
- 対応する Requirement と Design を再確認した
- 承認済み Task の範囲だけを変更した
- テストが実装前に期待した理由で失敗することを確認した
- 対象テストと関連する回帰テストを実行した
- 必要なドキュメントを更新した
- 完了した Task を `[x]` に更新した
- 仕様変更が必要になった場合は実装を停止した

## Verification review

- 各 Acceptance Criteria を個別に検証した
- 対象テストと回帰テストが成功した
- lint、type check、build など必要なコマンドを実行した
- API またはスキーマの整合性を確認した
- セキュリティ上重要な変更を確認した
- 未実行の検証と理由を記録した
- 既存障害と今回の変更による障害を区別した

## Reconciliation review

- 実装がすべての Acceptance Criteria を満たしている
- 要求されていない振る舞いを追加していない
- Design と実装が一致している
- `tasks.md` と実際の進捗が一致している
- Requirement → Design → Task → Test の対応が維持されている
- 必要なドキュメントが更新されている
- 未解決事項、制約、残存リスクを記録した
- 仕様と実装の不一致が残っていない
