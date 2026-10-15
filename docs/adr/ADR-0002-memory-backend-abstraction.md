# ADR-0002: Memory Backend を抽象化し MILLEFEUILLE v0.1.0 Adapter を提供する

- Status: Accepted
- Date: 2026-10-14

## Context

Une Feuille の目的は、利用側（NAVETTE / Imagorium）が特定の Memory Backend に
依存せず、共通 API から記憶を保存・取得・検索できるようにすることである。

MILLEFEUILLE v0.1.0 の公開 API は以下の制約を持つ（調査により確認済み）。

- append-only。update / delete API は存在しない。
- 保存は冪等。同一 `event_id` + 同一内容は元の `Receipt` を返し、同一 `event_id` +
  異なる内容は `ErrConflict` を返す。
- payload は valid JSON 必須、最大 1 MiB。`event_id` は呼び出し側が採番する。
- 検索は `source_id` / `event_type` / `received_at` 範囲によるメタデータフィルタのみ。
  semantic 検索・embedding・cursor pagination は未実装。
- 認証・認可を行わない trusted-caller モデル。`visibility` は単なるラベル。

この制約の上に Backend 非依存の共通 API を定義する必要がある。

## Decision

Une Feuille に Backend 非依存の `Store` インターフェースを定義し、最小 3 操作
（Save / Get / Search）を公開する。初期実装として MILLEFEUILLE v0.1.0 を利用する
Adapter を提供する。インターフェースの契約テスト（複数 Backend 実装に共通で適用可能な
テストスイート）を用意する。

- 検索（Search）は当面メタデータフィルタ検索に限定し、semantic 検索は MVP 範囲外とする。
- `event_id` 相当の ID は呼び出し側が供給する（MILLEFEUILLE の現仕様を踏襲）。

具体的な型・メソッドシグネチャは Une Feuille の Feature Spec（requirements / design）で
確定する。本 ADR は抽象化を行うという設計方針の記録にとどめる。

### 承認時の具体化事項（2026-10-14）

- 「Backend 非依存」とは、Une Feuille の共通契約（`Store` インターフェースと冪等・
  競合・順序・境界の意味論）を満たす Backend を交換できることを意味する。Backend ごとの
  差異は Adapter に封じ、利用側は共通契約のみに依存する。

## Rationale

- 最小 3 操作は MILLEFEUILLE の既存操作（`AppendObservation` / `GetObservation` /
  `QueryObservations`）へ素直に写像でき、Adapter 実装が薄くなる。
- Search をメタデータフィルタに限定することで、MILLEFEUILLE が持たない semantic 検索を
  無理に抽象化せず、将来 Backend が semantic 検索を提供したら拡張できる余地を残す。
- 契約テストにより、将来の別 Backend（例: Arctic Tern）が同一契約を満たすことを機械的に
  検証できる。
- 代替案 (A) semantic 検索の口だけ用意し未実装エラーを返す：API 面を先に広げるが、
  利用側が未実装前提のコードを書くリスクがあるため MVP では採らない。
- 代替案 (B) MILLEFEUILLE の型をそのまま公開する：Backend 非依存の目的に反するため不採用。

## Consequences

- 利点：利用側が Backend 実装を差し替え可能になる。契約テストで実装間の整合を担保できる。
- 制約：MILLEFEUILLE の append-only・冪等・JSON payload・ID 呼び出し側採番の制約が
  Une Feuille の API にも波及する（delete 不可、ID 必須）。
- 将来の影響：semantic 検索や別 Backend の追加時に `Store` インターフェースの拡張を
  要する可能性があり、その際は本 ADR を更新または後続 ADR を起こす。
