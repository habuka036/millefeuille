---
name: sdd-development
description: Enforce specification-driven development for feature development, behavior changes, API or data-model changes, architectural changes, refactoring with behavioral risk, and non-trivial bug fixes. Use when a request should follow Requirements, Design, Tasks, Implementation, Verification, and Reconciliation. Also trigger for Japanese requests mentioning SDD、仕様駆動開発、要件定義、設計、機能追加、仕様変更、または非自明なバグ修正. Do not use for read-only explanation, simple investigation, formatting-only edits, comments, or typo fixes.
---

# SDD Development

仕様を実装の前提および検証基準として扱う。

仕様成果物を作っただけで SDD を実施したと判断してはならない。要求、設計、タスク、コード、テストの対応関係を維持し、フェーズ間のゲートを厳守する。

## Fundamental rules

1. 要求を確定する前に設計を確定しない。
2. 設計を確定する前に実装タスクを確定しない。
3. 承認済みの実装タスクがない状態でプロダクションコードを変更しない。
4. 実装中に仕様の不足や矛盾を発見した場合は実装を停止する。
5. コードに合わせて事後的に仕様を書き換えない。
6. Requirement ID からテストまでのトレーサビリティを維持する。
7. 各フェーズの成果物を提示し、ユーザーの承認後に次へ進む。
8. 全フェーズの連続実行を許可された場合も、成果物とゲート検証を省略しない。
9. 重要な要求、制約、外部仕様を推測で補完しない。
10. 既存コードを正しい仕様の唯一の根拠とみなさない。

リポジトリ固有の `AGENTS.md` も遵守する。この Skill と競合する場合は、競合内容を示して作業を停止し、ユーザーへ判断を求める。

## Standard workflow

次の順序で実行する。

1. Discovery
2. Requirements または Bug Analysis
3. Design
4. Tasks
5. Implementation
6. Verification
7. Reconciliation

後続フェーズから前のフェーズを暗黙に補完しない。

## Determine the work type

### Feature Spec

以下を含む場合に使用する。

- 新機能
- 既存動作の変更
- API またはデータモデルの追加・変更
- 外部連携の追加・変更
- アーキテクチャの変更
- ユーザーが観測できる振る舞いの変更
- 複数コンポーネントへ影響するリファクタリング

成果物は `requirements.md`、`design.md`、`tasks.md` とする。

### Bugfix Spec

以下のいずれかに該当する場合に使用する。

- 現在の振る舞いが期待される振る舞いと異なる
- 原因調査が必要
- 修正によるリグレッションの可能性がある
- 複数ファイルまたは複数レイヤーへ影響する
- データ、認証、課金、権限、並行処理など重要領域に関係する

成果物は `bugfix.md`、`design.md`、`tasks.md` とする。

### Spec-exempt work

次の作業は、振る舞いや公開契約を変更しない場合に限り完全な Spec を省略できる。

- 読み取り専用の調査または説明
- コードレビュー
- コメント、誤字、書式だけの修正
- 意味を変えない機械的変更

省略可否が不明な場合は Spec が必要な作業として扱う。「簡単」「小さな変更」という表現だけを省略理由にしない。

## Discovery phase

コードを変更せず、次を確認する。

- リポジトリの `AGENTS.md`、README、開発ガイド
- 既存の仕様ディレクトリ
- 関連コード、テスト、API 定義
- 既存の設計パターンと検証コマンド
- 変更が影響する公開契約
- 既存の未コミット変更

作業種別、現在の振る舞い、変更対象、影響範囲、制約、不明点、仕様成果物の配置候補を整理する。

既存の仕様格納規約があれば従う。規約がなければ `specs/<spec-name>/` を提案する。新しい規約を導入する場合は、作成前にユーザーへ確認する。

## Requirements phase

Feature Spec では `requirements.md` を作成する。最低限、次を含める。

- Context
- Goals
- Non-goals
- Assumptions
- Constraints
- Requirements
- Non-functional requirements
- Compatibility requirements
- Security and privacy requirements
- Open questions

各要求に一意の ID を付ける。

- 機能要求: `REQ-001`
- 非機能要求: `NFR-001`
- セキュリティ要求: `SEC-001`
- 互換性要求: `COMP-001`

Acceptance Criteria は観測可能かつ検証可能な形式で記述する。

```text
WHEN <condition or event>
THE SYSTEM SHALL <observable behavior>
```

必要に応じて例外条件を記述する。

```text
IF <exceptional condition>
THEN THE SYSTEM SHALL <observable behavior>
```

「適切に」「必要に応じて」「高速に」「使いやすく」「十分に」「可能な限り」などの曖昧語には、具体的な条件または測定基準を併記する。

### Requirements gate

次を満たすまで Design へ進まない。

- Goals と Non-goals が明記されている
- 各要求に一意の ID と検証可能な Acceptance Criteria がある
- エラー条件と境界条件が定義されている
- 互換性への影響が確認されている
- 重要な未解決事項がない
- ユーザーが Requirements を承認している

完了後は変更内容を要約して停止し、承認、修正指示、または不明点への回答を求める。

## Bug analysis phase

Bugfix Spec では `bugfix.md` を作成し、次を含める。

- Summary
- Current behavior
- Expected behavior
- Unchanged behavior
- Reproduction conditions
- Evidence
- Suspected scope
- Root cause
- Regression risks
- Requirements and Acceptance Criteria
- Open questions

根本原因が未確認の場合は、`Confirmed evidence`、`Hypothesis`、`Unknown` を区別し、推測を事実として記述しない。

### Bug analysis gate

次を満たすまで Design へ進まない。

- 現在の振る舞いが再現または証拠で確認されている
- 期待される振る舞いと変更してはならない振る舞いが明記されている
- 原因が確認済みか未確認か明示されている
- 回帰リスクが整理されている
- Acceptance Criteria がある
- ユーザーが Bug Analysis を承認している

## Design phase

承認済みの Requirements または Bug Analysis を入力として `design.md` を作成し、次を含める。

- Requirement mapping
- Current architecture
- Proposed design
- Components and responsibilities
- Data flow
- API and data-model changes
- Error handling
- Security considerations
- Compatibility and migration
- Observability
- Test strategy
- Alternatives considered
- Risks and mitigations
- Open questions

設計上の判断に関連する Requirement ID を示す。API、データベース、認証、権限、課金、外部サービス、非同期処理への影響を明記する。

### Design gate

次を満たすまで Tasks へ進まない。

- すべての Requirement ID が設計へ対応付けられている
- コンポーネントの責務とデータフローが定義されている
- エラー処理とテスト戦略が定義されている
- 互換性と移行方法が確認されている
- リスクと軽減策が記載されている
- 重要な未解決事項がない
- ユーザーが Design を承認している

完了後は主要な設計判断と代替案を要約して停止する。

## Tasks phase

承認済み Design から `tasks.md` を作成する。タスクは独立して実行・検証できる大きさへ分割する。

各タスクに次を記載する。

- Task ID
- Requirement ID
- 関連する Design section
- 依存タスク
- 変更予定ファイル
- 完了条件
- 検証方法

例:

```markdown
- [ ] TASK-001 Add a failing test for the required behavior
  - Requirements: REQ-001
  - Design: Test strategy
  - Depends on: None
  - Files: <expected files>
  - Completion criteria:
    - Test fails for the expected reason
```

可能な場合はテストを先に追加する。「実装する」「対応する」だけでなく、観測可能な終了状態を完了条件にする。

`tasks.md` に次のトレーサビリティ表を含める。

```markdown
| Requirement | Design | Task | Test |
|---|---|---|---|
| REQ-001 | Proposed design | TASK-001 | <test name> |
```

### Tasks gate

次を満たすまで Implementation へ進まない。

- すべての要求が1つ以上のタスクへ対応している
- 各タスクに完了条件と依存関係がある
- テストタスクが含まれている
- 必要なドキュメント更新タスクが含まれている
- タスクが実行可能な大きさに分割されている
- ユーザーが Tasks を承認している

完了後は実行順序と依存関係を要約して停止する。

## Implementation phase

承認済み `tasks.md` に従って実装する。状態記号は次を使用する。

- `[ ]` 未着手
- `[~]` 実行中
- `[x]` 完了
- `[!]` ブロック中

原則として同時に複数タスクを実行中にしない。並列実行を明示的に許可され、依存関係がない場合のみ例外とする。

各タスクを次の順序で実行する。

1. Requirement と Design を再確認する
2. 必要なテストを追加・更新する
3. 期待した理由でテストが失敗することを確認する
4. 最小限の実装を行う
5. 対象テストと関連する回帰テストを実行する
6. ドキュメントを更新する
7. `tasks.md` を実際の状態に更新する

承認済みタスクに含まれない変更が必要になった場合は実装を停止し、Requirements、Design、または Tasks の適切なフェーズへ戻る。

## Verification phase

Acceptance Criteria ごとに検証する。適用可能な次の項目を実行する。

- 対象テストと関連する回帰テスト
- リポジトリ指定の検証コマンド
- lint、type check、build
- API またはスキーマ整合性確認
- ドキュメント更新確認
- セキュリティ上重要な変更の確認

実行できない検証は、未実行項目と理由を明示する。テスト失敗状態で完了と報告しない。既存の無関係な失敗と今回の変更による失敗を区別する。

## Reconciliation phase

実装と仕様成果物を比較し、次を確認する。

- すべての Acceptance Criteria を満たしている
- 要求されていない振る舞いを追加していない
- Design と実装が一致している
- `tasks.md` と実際の進捗が一致している
- Requirement → Design → Task → Test の対応が維持されている
- 必要なドキュメントが更新されている
- 未解決事項が記録されている

不一致があれば完了とせず、実装修正、仕様変更の承認依頼、またはブロッカー報告のいずれかを行う。

## Completion report

完了報告に次を含める。

- 実装した Requirement ID
- 完了した Task ID
- 主な変更
- 実行した検証
- Acceptance Criteria の検証結果
- 未実行の検証
- 既知の制約と残存リスク
- 仕様と実装の不一致の有無

## Approval interpretation

`OK`、`承認`、`進めて`、`問題ありません`、`次へ` は、現在提示しているフェーズだけの承認として扱う。残りの全フェーズに対する包括承認と解釈しない。

次のような明示指示がある場合のみ、フェーズ間の停止を省略できる。

- 全フェーズを承認なしで進めてください
- Requirements から実装まで連続して実行してください
- 途中確認は不要です

停止を省略しても、各ゲートを検証し、仕様成果物をすべて作成する。

## Existing specifications

既存仕様がある場合は最初から作り直さず、Status、最終更新日、現在のコードとの一致、未完了タスク、未解決事項、Requirement の網羅性、承認状態を確認する。

承認状態を判断できない場合は実装前にユーザーへ確認する。

## Prohibited behavior

- Requirements 作成と実装を同時に行う
- Design または Tasks を省略して実装する
- 未承認の要求を推測で実装する
- 実装後に都合よく Acceptance Criteria を変更する
- テスト結果を確認せず成功と報告する
- `tasks.md` を実際と異なる完了状態にする
- 未解決事項を暗黙に無視する
- 既存コードだけを根拠にその挙動を正しい仕様とする
- ユーザーの「実装して」を全フェーズの暗黙承認として扱う
