# Une Feuille — Requirements

- Status: Approved (conditions reflected 2026-10-14); Design in progress
- Date: 2026-10-14
- Spec location: `millefeuille/docs/specs/unefeuille/`
- Related ADRs: ADR-0001 (placement, Accepted), ADR-0002 (abstraction, Accepted)

## Introduction

Une Feuille は、利用側（NAVETTE / Imagorium）が特定の Memory Backend に強く依存せず、
共通 API から記憶を保存・取得・検索できるようにする抽象化レイヤである。MILLEFEUILLE
リポジトリ内の Go パッケージ `github.com/habuka036/millefeuille/unefeuille` として実装し、
初期 Backend として MILLEFEUILLE v0.1.0 を利用する Adapter を提供する。

この requirements は Backend 非依存の `Store` 契約、MILLEFEUILLE Adapter、契約テスト、
および NAVETTE が Une Feuille 未完成でも動けるようにする in-memory fake の範囲を定義する。

## Goals

- Backend 非依存の最小 Memory API（保存・取得・検索）を定義する。
- MILLEFEUILLE v0.1.0 を利用する Adapter を提供する。
- 複数 Backend 実装に共通で適用できる契約テストスイートを提供する。
- NAVETTE が依存できる in-memory fake 実装を提供する。

## Non-goals

- semantic 検索・embedding・ベクトル検索（MILLEFEUILLE に存在せず、MVP 範囲外）。
- update / delete API（MILLEFEUILLE は append-only）。
- 認証・認可（trusted-caller モデルを踏襲。利用側境界の責務）。
- cursor pagination（MILLEFEUILLE v0.1.0 で未実装）。
- 新規 Backend（Arctic Tern 等）の実装。契約テストで将来の受け入れ余地のみ用意する。

## Assumptions

- MILLEFEUILLE v0.1.0 の公開 API（`Open` / `Store` / `AppendObservation` /
  `GetObservation` / `QueryObservations`）と意味論は調査済みで安定している。
- 記憶の ID は呼び出し側が採番する（MILLEFEUILLE の `event_id` 現仕様を踏襲）。
- payload は valid JSON、最大 1 MiB（MILLEFEUILLE 制約を踏襲）。

## Constraints

- append-only。保存した記憶の更新・削除はできない。
- 保存は冪等。同一 ID + 同一内容は元の結果を返し、同一 ID + 異なる内容は競合エラー。
- 検索はメタデータフィルタのみ（source / type / 時間範囲 / 件数上限）。
- 検索フィルタのフィールド名は MILLEFEUILLE 固有名ではなく Backend 非依存の中立名を用いる。
- 件数上限 `Limit=0` の既定値 100 は Une Feuille の共通契約として全 Backend に適用する。
- すべての共通エラーは `errors.Is` で判定可能な sentinel として定義する。
- 検索結果の順序は確定時刻の降順。同一確定時刻の場合も ID による決定的タイブレークを行う。
- ドメイン型は Une Feuille 独自の公開型とし、MILLEFEUILLE 型の直接 alias は用いない。
  Backend 固有型との変換は Adapter が担う。
- Go 1.26 系。追加依存は原則入れない（MILLEFEUILLE の方針を踏襲）。

## Requirements

### Requirement 1: Backend 非依存の記憶保存

**User Story:** 利用者として、Backend 実装を意識せず記憶を保存したい。再送時に重複が
生じないことを期待する。

#### Acceptance Criteria

1. WHEN 呼び出し側が ID・種別・ソース・観測時刻・JSON payload を含む記憶を Save する
   THE SYSTEM SHALL その記憶を Backend に永続化し、ID と確定時刻を含む保存結果を返す。
2. WHEN 同一 ID かつ同一内容の記憶が再度 Save される
   THE SYSTEM SHALL 新規保存を行わず、最初の保存結果と同一の結果を返す（冪等）。
3. IF 同一 ID かつ内容が異なる記憶が Save される
   THEN THE SYSTEM SHALL 競合を示すエラー（`errors.Is` で判定可能）を返し、新規保存を行わない。
4. IF 必須フィールド（ID・種別・ソース・発生経路(Origin)・スキーマ版(SchemaVersion)・観測時刻・payload）が欠落している
   THEN THE SYSTEM SHALL 不正入力エラーを返す。
   （必須集合 = {ID, Kind, Source, Origin, SchemaVersion, ObservedAt, Content}。Device は任意。
   Origin / SchemaVersion 必須の根拠は MILLEFEUILLE `validateInput` が provenance.origin と
   schema_version を必須とすることによる。契約を全 Backend で一致させるため Une Feuille で
   統一して必須とする。design.md §Origin / 実装 TASK-009 参照。）
5. IF payload が valid JSON でない、または 1 MiB を超える
   THEN THE SYSTEM SHALL 不正入力エラー（サイズ超過は区別可能なエラー）を返す。

### Requirement 2: 記憶の取得

**User Story:** 利用者として、ID を指定して保存済みの記憶を取得したい。

#### Acceptance Criteria

1. WHEN 存在する ID で Get する
   THE SYSTEM SHALL その記憶の全フィールド（確定時刻含む）を返す。
2. IF 存在しない ID で Get する
   THEN THE SYSTEM SHALL 「見つからない」エラー（`errors.Is` で判定可能）を返す。

### Requirement 3: メタデータフィルタによる検索

**User Story:** 利用者として、ソース・種別・時間範囲で記憶を絞り込み、件数を制限して
取得したい。

#### Acceptance Criteria

1. WHEN ソース・種別・時間範囲・件数上限を指定して Search する
   THE SYSTEM SHALL 条件に一致する記憶を確定時刻の降順で、上限件数まで返す。
   同一確定時刻の記憶は ID による決定的タイブレークで安定した順序とする。
2. WHEN 件数上限に 0 を指定する
   THE SYSTEM SHALL Une Feuille 共通契約の既定上限 100 を適用する（全 Backend 共通）。
3. IF 件数上限に負値を指定する
   THEN THE SYSTEM SHALL 不正フィルタエラーを返す。
4. WHEN 条件に一致する記憶が存在しない
   THE SYSTEM SHALL 空の結果を返す（エラーにしない）。
5. WHEN 検索フィルタのフィールドを指定する
   THE SYSTEM SHALL Backend 非依存の中立的フィールド名で指定を受け付ける
   （MILLEFEUILLE 固有名を公開 API に露出しない）。

### Requirement 4: MILLEFEUILLE v0.1.0 Adapter

**User Story:** 統合担当として、MILLEFEUILLE v0.1.0 を Backend とする Une Feuille 実装を
使いたい。

#### Acceptance Criteria

1. WHEN MILLEFEUILLE の `Store` を与えて Une Feuille Adapter を構築する
   THE SYSTEM SHALL `Store` 契約を満たす Adapter を返す。
2. WHEN Adapter 経由で Save / Get / Search を呼ぶ
   THE SYSTEM SHALL MILLEFEUILLE の `AppendObservation` / `GetObservation` /
   `QueryObservations` にそれぞれ写像し、MILLEFEUILLE の sentinel error を Une Feuille の
   対応エラーに変換して返す。

### Requirement 5: 契約テストスイート

**User Story:** 開発者として、任意の `Store` 実装が同じ契約を満たすことを機械的に検証したい。

#### Acceptance Criteria

1. WHEN 契約テストに `Store` 実装のコンストラクタを与えて実行する
   THE SYSTEM SHALL Requirement 1〜3 の受け入れ基準（冪等・競合・不正入力・取得・検索・
   境界）を当該実装に対して検証する。
2. WHEN MILLEFEUILLE Adapter と in-memory fake の双方に契約テストを適用する
   THE SYSTEM SHALL 両実装がテストにパスする。

### Requirement 6: In-memory fake 実装

**User Story:** NAVETTE 開発者として、MILLEFEUILLE をファイルで用意せずとも Une Feuille の
`Store` を使ってタスク実行を動かしたい。

#### Acceptance Criteria

1. WHEN in-memory fake を構築する
   THE SYSTEM SHALL ファイルや外部依存なしに `Store` 契約を満たす実装を返す。
2. WHEN fake に対して Save / Get / Search を呼ぶ
   THE SYSTEM SHALL MILLEFEUILLE Adapter と同じ契約（冪等・競合・順序・境界）で振る舞う。
3. THE in-memory fake SHALL 契約テストおよび統合テスト用途に限定され、NAVETTE の
   SQLite 永続化を置き換える本番用途には用いない。

### Requirement 7: 共通エラーの定義

**User Story:** 利用者として、Backend に依存せず `errors.Is` で記憶操作のエラー種別を
判定したい。

#### Acceptance Criteria

1. THE SYSTEM SHALL 「見つからない」「競合」「不正入力」「payload サイズ超過」「不正フィルタ」
   に対応する sentinel エラーを Une Feuille パッケージに公開する。
2. WHEN Adapter が Backend 固有エラー（MILLEFEUILLE の sentinel 等）を受け取る
   THE SYSTEM SHALL それを対応する Une Feuille の sentinel エラーに変換し、`errors.Is` で
   判定可能にする。

## Non-functional requirements

- NFR-001: 追加の外部依存を導入しない（標準ライブラリと MILLEFEUILLE 既存依存の範囲）。
- NFR-002: Save / Get / Search は `context.Context` を受け取り、キャンセル・デッドラインを
  尊重する。`Close()` はリソース解放操作のため ctx を取らない（design.md §Close と Context
  の NFR 整合）。
- NFR-003: `go build ./...` / `go test ./...` / `go vet ./...` がパスする。

## Compatibility requirements

- COMP-001: MILLEFEUILLE v0.1.0 の既存公開 API を変更しない（Une Feuille は新規パッケージ
  として追加し、既存 `millefeuille` パッケージの契約を破壊しない）。

## Security and privacy requirements

- SEC-001: Une Feuille 自身は認証・認可を行わない（trusted-caller モデルを明記）。
  `visibility` 等のラベルはフィルタに用いず、verbatim に保存・返却する。
- SEC-002: trust / provenance を自動で推論・昇格しない（MILLEFEUILLE の方針を踏襲）。

## Resolved decisions (承認時に確定, 2026-10-14)

- RD-1 (旧 OQ-1): Une Feuille は独自の公開ドメイン型を定義する。MILLEFEUILLE 型の直接
  alias は用いず、Adapter で相互変換する。
- RD-2 (旧 OQ-2): `visibility` / `provenance` の情報は失わずに保持・返却する。ただし
  Une Feuille 自身は認証・認可を提供せず、これらラベルをアクセス制御として扱わない
  （SEC-001 と整合）。
- RD-3 (旧 OQ-3): 検索フィルタは Backend 非依存の中立的フィールド名を採用し、MILLEFEUILLE
  固有名を公開 API に露出しない（Requirement 3 AC-5）。

具体的なフィールド名・型レイアウト・変換規則は design.md で確定する。
