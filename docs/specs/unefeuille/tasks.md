# Une Feuille — Tasks

- Status: Draft (awaiting review)
- Date: 2026-10-14
- Spec location: `millefeuille/docs/specs/unefeuille/`
- Inputs: `requirements.md` (Approved w/ conditions), `design.md` (revised, awaiting review)

実装順序の原則：

- 既存 MILLEFEUILLE 公開 API・`internal/*`・migrations を変更しない（COMP-001）。
- 小さく独立して実装・検証できる単位に分割。可能な限りテスト先行。
- `unefeuille`（契約・型・エラー・fake・契約テスト）を先に完成させ、最後に MILLEFEUILLE
  Adapter を接続する。これにより Adapter 実装前に契約が fake で固まる。
- 各タスク完了時に `go build ./...` / `go vet ./...` と対象テストを実行。

状態記号：`[ ]` 未着手 / `[~]` 実行中 / `[x]` 完了 / `[!]` ブロック。

## Tasks

- [x] TASK-001 公開ドメイン型と Store インターフェースのスケルトン
  - Requirements: R1, R2, R3（型の土台）
  - Design: Public types / Store インターフェース
  - Depends on: None
  - Files: `unefeuille/memory.go`, `unefeuille/store.go`
  - Completion criteria:
    - `Memory` / `Labels` / `SaveReceipt` / `Query` と `Store` インターフェースを定義。
    - `SaveReceipt` に `ContentHash` を含めない（OQ-A）。`Memory.Device` を含む。
    - `Close()` は ctx を取らない。`go build ./...` が通る。

- [x] TASK-002 共通 sentinel エラーの定義
  - Requirements: R7
  - Design: Errors
  - Depends on: TASK-001
  - Files: `unefeuille/errors.go`
  - Completion criteria:
    - `ErrNotFound` / `ErrConflict` / `ErrInvalidMemory` / `ErrContentTooLarge` /
      `ErrInvalidQuery` を公開。`errors.Is` で判定可能。

- [x] TASK-003 入力検証ヘルパ（Save 用）
  - Requirements: R1 (AC-4, AC-5)
  - Design: Error handling / §Origin
  - Depends on: TASK-001, TASK-002
  - Files: `unefeuille/validate.go`, `unefeuille/validate_test.go`
  - Completion criteria:
    - 必須集合 {ID, Kind, Source, Origin, ObservedAt, Content} の欠落で `ErrInvalidMemory`。
    - 非 JSON の Content で `ErrInvalidMemory`、>1 MiB で `ErrContentTooLarge`。
    - テストが期待どおり失敗→実装後にパス。

- [x] TASK-004 契約テストスイート（実装非依存）
  - Requirements: R5（R1〜R3, R7 の受け入れ基準を検証）
  - Design: contracttest / Test strategy
  - Depends on: TASK-001, TASK-002, TASK-003
  - Files: `unefeuille/contracttest/contract.go`
  - Completion criteria:
    - `Run(t *testing.T, newStore Factory)` を公開。
    - 外部観測可能な振る舞いのみ検証：Save→Get ラウンドトリップ（Device/Labels 保持）、
      JSON 同一性（意味論比較）、時刻精度（StoredAt 一貫）、冪等（同一 StoredAt・重複なし）、
      競合（元記憶保持）、不正入力各種、NotFound、Search フィルタ/Limit 丸め/空、
      同一時刻の ID 降順決定性、キャンセル時に副作用なし（Save 後 Get で NotFound）、
      Close 冪等。
    - 内部ハッシュ等の実装詳細に依存しない。

- [x] TASK-005 in-memory fake 実装（memfake）
  - Requirements: R6
  - Design: memfake
  - Depends on: TASK-001, TASK-002, TASK-003
  - Files: `unefeuille/memfake/memfake.go`
  - Completion criteria:
    - 外部依存なしに `Store` を満たす。単調増加クロックで `StoredAt` 付与。
    - 内容同一性は内部判定（アルゴリズム自由, OQ-B）。`Close()` は no-op。
    - ctx キャンセルを各メソッドで尊重。

- [x] TASK-006 契約テストを memfake に適用（fake の契約適合を確認）
  - Requirements: R5 (AC-2), R6
  - Design: Test strategy
  - Depends on: TASK-004, TASK-005
  - Files: `unefeuille/memfake/memfake_test.go`
  - Completion criteria:
    - `contracttest.Run(t, func(t) Store { return memfake.New() })` が全ケースをパス。

- [x] TASK-007 MILLEFEUILLE Adapter（millefeuilleadapter）
  - Requirements: R4
  - Design: millefeuilleadapter / Save・Get・Search・Error mapping / §Store の所有権と Close
  - Depends on: TASK-001, TASK-002, TASK-003
  - Files: `unefeuille/millefeuilleadapter/adapter.go`
  - Completion criteria:
    - `New(st millefeuille.Store) unefeuille.Store`（借用、Close は内部 Store を閉じない）。
    - `Open(ctx, path, ...) (unefeuille.Store, error)`（所有、Close は内部 Store を閉じる、
      Close 冪等）。
    - §フィールド保持表どおりに Memory⇔InputObservation/Observation を変換（Device 含む）。
    - Query→Filter 変換。MILLEFEUILLE sentinel → unefeuille sentinel を `%w` で変換。
    - Save 前に TASK-003 の検証を適用（Origin 空は Adapter 到達前に ErrInvalidMemory）。

- [x] TASK-008 契約テストを Adapter に適用（統合）
  - Requirements: R4, R5 (AC-2)
  - Design: Test strategy
  - Depends on: TASK-004, TASK-007
  - Files: `unefeuille/millefeuilleadapter/adapter_test.go`
  - Completion criteria:
    - `t.TempDir()` の実ファイル SQLite を用い `contracttest.Run` が全ケースをパス。
    - memfake と Adapter が同一の観測挙動を示す。

- [x] TASK-009 Adapter 固有の変換単体テスト
  - Requirements: R4
  - Design: Save/Get/Search/Error mapping, §フィールド保持表
  - Depends on: TASK-007
  - Files: `unefeuille/millefeuilleadapter/mapping_test.go`
  - Completion criteria:
    - 各フィールドの往復保持（特に Device・Labels・Origin）を固定。
    - 各 MILLEFEUILLE sentinel が対応する unefeuille sentinel に変換される。

- [x] TASK-010 パッケージドキュメントと trusted-caller 注記
  - Requirements: SEC-001, SEC-002
  - Design: Security considerations
  - Depends on: TASK-001
  - Files: `unefeuille/doc.go`
  - Completion criteria:
    - 認証・認可を行わない旨、`Labels` をアクセス制御に使わない旨、trust 非推論を明記。

- [x] TASK-011 ドキュメント整合（requirements/design と実装の一致, 非回帰確認）
  - Requirements: COMP-001, NFR-003, R-4(design risk)
  - Design: Compatibility / Reconciliation 相当
  - Depends on: TASK-001〜TASK-010
  - Files: `docs/specs/unefeuille/requirements.md`, `docs/specs/unefeuille/design.md`
  - Completion criteria:
    - 必須フィールド集合・NFR-002 範囲・用語が実装と一致。
    - `go build ./...` / `go test ./...` / `go vet ./...` が全てパス。
    - 既存 MILLEFEUILLE テスト（ルート・internal/*）が非回帰で green。

## Traceability

| Requirement | Design | Task | Test |
|---|---|---|---|
| R1 保存/冪等/競合/不正入力 | Store.Save, Error handling, §Origin | TASK-001,003,007 | contract: save/idempotent/conflict/invalid |
| R2 取得/NotFound | Store.Get, Get mapping | TASK-001,007 | contract: get/not-found |
| R3 検索/中立名/順序/既定100 | Store.Search, Search mapping | TASK-001,007 | contract: search/limit/order |
| R4 MILLEFEUILLE Adapter | millefeuilleadapter, mapping tables | TASK-007 | TASK-008, TASK-009 |
| R5 契約テスト | contracttest | TASK-004 | TASK-006, TASK-008 |
| R6 in-memory fake | memfake | TASK-005 | TASK-006 |
| R7 共通 sentinel | Errors, Error mapping | TASK-002 | contract + TASK-009 |
| NFR-002 ctx 尊重 | Store インターフェース, §Close×ctx | TASK-005,007 | contract: cancel |
| COMP-001 既存 API 不変 | Compatibility | TASK-011 | 既存 MF テスト非回帰 |
| SEC-001/002 | Security considerations | TASK-010 | doc レビュー |

## 実行順序まとめ

1. TASK-001 → 002 → 003（型・エラー・検証の土台）
2. TASK-004（契約テスト）と TASK-005（fake）→ TASK-006（fake で契約確認）
3. TASK-007（Adapter）→ TASK-008（Adapter 契約）→ TASK-009（変換単体）
4. TASK-010（doc）→ TASK-011（整合・非回帰）

TASK-004 と TASK-005 は TASK-003 完了後に並行可能（依存なし）。それ以外は上記順序。
