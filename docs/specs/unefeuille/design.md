# Une Feuille — Design

- Status: Draft (awaiting review)
- Date: 2026-10-14
- Spec location: `millefeuille/docs/specs/unefeuille/`
- Inputs: `requirements.md` (Approved w/ conditions), ADR-0001 (Accepted), ADR-0002 (Accepted)

## Requirement mapping

| Requirement | Design section |
|---|---|
| R1 保存（冪等・競合・不正入力） | Public API §Store.Save, Adapter §Save mapping, Errors |
| R2 取得（NotFound） | Public API §Store.Get, Adapter §Get mapping |
| R3 検索（中立フィルタ・順序・既定100） | Public API §Query/§Store.Search, Ordering |
| R4 MILLEFEUILLE Adapter | Components §millefeuilleadapter |
| R5 契約テスト | Components §contract test suite |
| R6 In-memory fake | Components §memfake |
| R7 共通エラー sentinel | Errors |
| NFR-001 依存なし | Dependencies |
| NFR-002 context 尊重 | Public API（全メソッド ctx 受領） |
| COMP-001 既存 API 不変 | Compatibility |
| SEC-001/002 認可なし・trust 非推論 | Security considerations |

## Current architecture（再掲・確認済み）

MILLEFEUILLE v0.1.0 ルートパッケージ `millefeuille` が公開：

- `Open(ctx, path, ...Option) (Store, error)` — file-based SQLite のみ。
- `Store.AppendObservation(ctx, InputObservation) (Receipt, error)` — append-only、冪等、
  同一 ID 異内容で `ErrConflict`。
- `Store.GetObservation(ctx, eventID) (*Observation, error)` — `ErrNotFound`。
- `Store.QueryObservations(ctx, Filter) (Page, error)` — `received_at DESC, event_id DESC`、
  `Limit` 0→100 / >1000→1000 / <0→`ErrInvalidFilter`。
- sentinels: `ErrNotFound` / `ErrConflict` / `ErrInvalidObservation` /
  `ErrPayloadTooLarge` / `ErrInvalidFilter`。

Une Feuille はこの上に Backend 非依存レイヤを重ねる。既存コードは一切変更しない。

## Proposed design

### パッケージ構成

```
millefeuille/
  unefeuille/                      ← 公開パッケージ（Backend 非依存の契約と型）
    memory.go                      ← Memory, SaveReceipt ドメイン型
    store.go                       ← Store インターフェース, Query 型
    errors.go                      ← 共通 sentinel エラー
    memfake/                       ← in-memory fake 実装（テスト/統合用）
      memfake.go
    millefeuilleadapter/           ← MILLEFEUILLE v0.1.0 Adapter
      adapter.go
    contracttest/                  ← 契約テストスイート（実装非依存）
      contract.go
    unefeuille_test.go             ← 契約テストを memfake に適用
    millefeuilleadapter/adapter_test.go ← 契約テストを Adapter に適用
```

Backend 非依存の契約（`unefeuille`）と Backend 固有実装（`millefeuilleadapter`）を
分離し、`unefeuille` は MILLEFEUILLE ルートパッケージを import しない（依存方向は
adapter → unefeuille、adapter → millefeuille の一方向）。これにより RD-1（独自型、
alias 回避）と「Backend 交換可能」を型レベルで担保する。

### 公開型（RD-1: 独自ドメイン型）

```go
package unefeuille

import (
    "encoding/json"
    "time"
)

// Memory は Une Feuille が扱う記憶の 1 単位。Backend 非依存の独自型。
type Memory struct {
    ID         string          // 呼び出し側が採番（空は不正入力）
    Kind       string          // 記憶の種別（MILLEFEUILLE の event_type に相当）
    Source     string          // 記憶の発生元（source_id に相当）
    Device     string          // 発生デバイス（任意; MILLEFEUILLE device_id に相当）
    Origin     string          // 発生経路（provenance.origin に相当; 下記 §Origin 参照）
    ObservedAt time.Time       // 観測時刻（呼び出し側指定, UTC 正規化）
    StoredAt   time.Time       // 確定時刻（保存時に Backend が付与、Save 入力では無視）
    Content    json.RawMessage // valid JSON, 最大 1 MiB
    Labels     Labels          // visibility / provenance.trust 等のラベル（RD-2）
}

// Labels は記憶に付随するメタラベル。アクセス制御には用いない（SEC-001）。
type Labels struct {
    Visibility string // 既定 "private"。verbatim 保存、フィルタに使わない。
    TrustLevel string // 既定 "unverified"。自動昇格しない（SEC-002）。
    SchemaVersion string // 記憶内容のスキーマ版（MILLEFEUILLE schema_version 相当）
}

// SaveReceipt は Save 成功時の結果。
type SaveReceipt struct {
    ID       string
    StoredAt time.Time
    // 注: Backend 固有の内容ハッシュは MVP の公開契約に含めない（OQ-A）。
    // 利用側は Backend 固有ハッシュ値に依存しない。可観測性が必要になった段階で、
    // Backend 非依存の「不透明識別子」として後方互換に追加しうる。
}

// Query は Backend 非依存の中立的検索フィルタ（RD-3）。
type Query struct {
    Source string    // 空＝制約なし
    Kind   string    // 空＝制約なし
    From   time.Time // StoredAt 下限（含む）, zero＝制約なし
    To     time.Time // StoredAt 上限（含む）, zero＝制約なし
    Limit  int       // 0＝既定 100（共通契約）, >1000＝1000 に丸め, <0＝ErrInvalidQuery
}
```

命名方針：MILLEFEUILLE 固有名（`event_type` / `source_id` / `received_at`）を公開
API に出さず、中立名（`Kind` / `Source` / `StoredAt` / `From` / `To`）を採用（RD-3）。
`Memory` に `visibility` / `provenance` の情報を `Labels` として保持し、情報欠落を防ぐ
（RD-2）。ただし Une Feuille はこれらをフィルタ条件に使わない。

### Store インターフェース

```go
package unefeuille

import "context"

type Store interface {
    // Save は記憶を永続化する。ID+内容が同一なら冪等（元の SaveReceipt を返す）。
    // ID 同一で内容が異なる場合は ErrConflict。
    Save(ctx context.Context, m Memory) (SaveReceipt, error)

    // Get は ID で記憶を取得する。無ければ ErrNotFound。
    Get(ctx context.Context, id string) (Memory, error)

    // Search はフィルタに一致する記憶を StoredAt 降順（同時刻は ID 降順でタイブレーク）
    // で、Limit 件まで返す。一致なしは空スライス + nil error。
    Search(ctx context.Context, q Query) ([]Memory, error)

    // Close は Backend リソースを解放する。
    Close() error
}
```

全メソッドは `ctx` を第一引数に取り、キャンセル・デッドラインを尊重（NFR-002）。
ただし `Close()` は `ctx` を取らない（下記 §Store の所有権と Close 参照）。

### MILLEFEUILLE フィールドの保持 / 対象外（調査結果）

MILLEFEUILLE `InputObservation` / `Observation`（`internal/model/model.go`）の全フィールドに
対する Adapter の扱いを以下に明示する。Adapter 変換で情報が失われないことを保証する。

| MILLEFEUILLE フィールド | Une Feuille での保持先 | 区分 | 備考 |
|---|---|---|---|
| EventID | `Memory.ID` | 保持 | 呼び出し側採番 |
| SchemaVersion | `Memory.Labels.SchemaVersion` | 保持 | 記憶内容のスキーマ版 |
| SourceID | `Memory.Source` | 保持 | |
| DeviceID | `Memory.Device` | 保持 | MILLEFEUILLE では任意。空許容 |
| EventType | `Memory.Kind` | 保持 | |
| ObservedAt | `Memory.ObservedAt` | 保持 | UTC 正規化 |
| ReceivedAt | `Memory.StoredAt` | 保持（読み取り専用） | Save 入力では無視、Get/Search で復元 |
| Payload | `Memory.Content` | 保持 | verbatim（valid JSON, ≤1 MiB） |
| Visibility | `Memory.Labels.Visibility` | 保持 | ラベル。フィルタ不可（SEC-001） |
| Provenance.Origin | `Memory.Origin` | 保持 | 必須（下記 §Origin） |
| Provenance.TrustLevel | `Memory.Labels.TrustLevel` | 保持 | 自動昇格しない（SEC-002） |
| （MILLEFEUILLE 内部）content_hash | — | 対象外 | Backend 内部。公開契約に出さない（OQ-A） |

→ 当初 design で欠落していた **DeviceID** を `Memory.Device` として追加し、保持対象に
含めた。公開契約から対象外とするのは MILLEFEUILLE 内部の `content_hash` のみ。

### Origin を必須とする根拠（Requirements と整合）

MILLEFEUILLE の `validateInput`（`append.go`）は `provenance.origin` を**必須**として扱い、
空の場合 `ErrInvalidObservation` を返す（調査により確認済み）。したがって Adapter が
MILLEFEUILLE へ保存する際、`Memory.Origin` が空だと必ず失敗する。

Une Feuille は Backend 非依存契約として、`Memory.Origin` を Save の**必須フィールド**とし、
空の場合は Adapter に到達する前に `ErrInvalidMemory` を返す。これにより memfake と Adapter
で観測挙動が一致する（Backend 固有の失敗に依存しない）。

requirements.md Requirement 1 AC-4 の必須フィールド集合を
`{ID, Kind, Source, Origin, SchemaVersion, ObservedAt, Content}` と確定する（Device は任意）。
**SchemaVersion も Origin と同じ理由で必須**：MILLEFEUILLE は `schema_version` を必須と
するため、実装時（TASK-009）に判明した契約差異を Une Feuille 契約側で吸収し、必須に統一した
（memfake / Adapter 双方が同一の観測挙動を示す）。

### Store の所有権と Close（責任の明確化）

- `millefeuilleadapter.Open(ctx, path, ...)`：Adapter が内部で `millefeuille.Open` を呼び、
  生成した `millefeuille.Store` を**所有する**。この場合 Adapter の `Close()` は内部
  `millefeuille.Store` を `Close` する責任を持つ。
- `millefeuilleadapter.New(st millefeuille.Store)`：呼び出し側が所有する `st` を**借用する**。
  Adapter はライフサイクルを所有せず、`Close()` は内部 Store を**閉じない**（借用元が閉じる）。
  借用/所有の違いはコンストラクタのドキュメントコメントで明示する。
- `memfake.New()`：外部リソースを持たない。`Close()` は冪等な no-op（nil 返却）。

二重 Close の安全性：`Open` 由来の Adapter は `Close()` を複数回呼んでも安全
（2 回目以降は no-op 相当）。契約テストで Close の冪等性を検証する。

### Close と Context の NFR 整合

NFR-002 は「API は ctx を受け取りキャンセル・デッドラインを尊重する」。`Close()` は
リソース解放操作であり、ctx によるキャンセルの対象にしない（解放の途中キャンセルは
リソースリークを招くため）。したがって `Close() error` は ctx を取らない。これは
MILLEFEUILLE `Store.Close() error` のシグネチャとも一致する。NFR-002 の適用範囲を
「Save / Get / Search」に限定する旨を requirements.md に注記する（Tasks で整合）。

### 共通エラー（R7, errors.Is 判定可能）

```go
package unefeuille

import "errors"

var (
    ErrNotFound        = errors.New("unefeuille: memory not found")
    ErrConflict        = errors.New("unefeuille: id exists with different content")
    ErrInvalidMemory   = errors.New("unefeuille: invalid memory")      // 必須欠落/不正JSON
    ErrContentTooLarge = errors.New("unefeuille: content exceeds 1 MiB")
    ErrInvalidQuery    = errors.New("unefeuille: invalid query")       // 負の Limit 等
)
```

これらは `unefeuille` パッケージ固有の値。Adapter が Backend エラーを変換して返すため、
利用側は Backend を問わず `errors.Is(err, unefeuille.ErrConflict)` 等で判定できる。

## Components and responsibilities

### millefeuilleadapter（R4）

MILLEFEUILLE `millefeuille.Store` をラップし `unefeuille.Store` を満たす。

```go
package millefeuilleadapter

func New(st millefeuille.Store) unefeuille.Store   // 既存 Store を注入
func Open(ctx, path string, ...) (unefeuille.Store, error) // millefeuille.Open の薄い包み
```

#### Save mapping

`unefeuille.Memory` → `millefeuille.InputObservation`:

| unefeuille.Memory | millefeuille.InputObservation |
|---|---|
| ID | EventID |
| Labels.SchemaVersion | SchemaVersion |
| Source | SourceID |
| Device | DeviceID |
| Kind | EventType |
| ObservedAt | ObservedAt |
| Content | Payload |
| Labels.Visibility | Visibility |
| Origin | Provenance.Origin |
| Labels.TrustLevel | Provenance.TrustLevel |

`StoredAt` は入力で無視し、戻り `Receipt.ReceivedAt` を `SaveReceipt.StoredAt` に写す。
MILLEFEUILLE 内部の `content_hash` は公開契約に出さない（OQ-A）。

#### Get mapping

`GetObservation` の `*Observation` → `Memory`（逆写像）。`ReceivedAt`→`StoredAt`。

#### Search mapping

`unefeuille.Query` → `millefeuille.Filter`:

| Query | Filter |
|---|---|
| Source | SourceID |
| Kind | EventType |
| From | From |
| To | To |
| Limit | Limit |

`Page.Items`（`[]Observation`）→ `[]Memory`。順序は MILLEFEUILLE が
`received_at DESC, event_id DESC` を保証するため、タイブレークはそのまま満たされる。

#### Error mapping

| millefeuille | unefeuille |
|---|---|
| ErrNotFound | ErrNotFound |
| ErrConflict | ErrConflict |
| ErrInvalidObservation | ErrInvalidMemory |
| ErrPayloadTooLarge | ErrContentTooLarge |
| ErrInvalidFilter | ErrInvalidQuery |

変換は `errors.Is(mfErr, millefeuille.ErrX)` で判定し、対応する `unefeuille.ErrY` を
`fmt.Errorf("...: %w", unefeuille.ErrY)` で包んで返す（元エラーの文脈はメッセージに残す）。

### memfake（R6）

外部依存なしの `unefeuille.Store` 実装。契約テスト・統合テスト専用（NAVETTE の SQLite
永続化を置換しない）。

- 内部表現：`map[string]storedMemory`（ID→内容 + 内容フィンガープリント + StoredAt）
  + `sync.RWMutex`。フィンガープリントは memfake 内部実装であり公開契約に出さない。
- Save：ID 既存かつ内容同一 → 既存の観測結果（冪等, 同一 `StoredAt`）。ID 既存で内容相違
  → `ErrConflict`（元の記憶を保持）。未存在 → `StoredAt` を単調増加クロックで付与し保存。
  入力検証（必須/JSON/サイズ）を実施。
- 内容同一性の判定：payload を MILLEFEUILLE と同じ正規化規則（`json.Decoder(UseNumber)`
  デコード→`json.Marshal` 再エンコード）で正規化し、主要フィールドと合わせて比較する。
  内部でハッシュ化するか直接比較するかは自由だが（OQ-B: ハッシュ算法の一致は不要）、
  正規化規則は MILLEFEUILLE に揃え、冪等/競合の**観測挙動**を Adapter と一致させる。
- Search：全件走査 → フィルタ → `StoredAt DESC, ID DESC` ソート → Limit 丸め（0→100,
  >1000→1000, <0→`ErrInvalidQuery`）。
- ctx：各メソッド冒頭で `ctx.Err()` を確認（NFR-002）。
- 決定的順序のため、同時刻でも ID 降順で安定ソート。

### contracttest（R5）

実装非依存の契約テストスイート。`testing.T` と `Store` ファクトリを受け取り、
Requirement 1〜3・7 の受け入れ基準を検証する。

```go
package contracttest

// Factory は被テスト Store を生成する。各サブテストで新しいインスタンスを得る。
type Factory func(t *testing.T) unefeuille.Store

// Run は与えた Factory の実装に全契約ケースを適用する。
func Run(t *testing.T, newStore Factory)
```

検証ケース（**外部観測可能な振る舞い**のみを検証。内部表現・ハッシュ算法には依存しない）：

- Save→Get ラウンドトリップで全フィールドが保持される（Labels・Device 含む, RD-2）。
- **JSON 同一性**：冪等/競合の判定基準は MILLEFEUILLE v0.1.0 の実際の規則に一致させる。
  MILLEFEUILLE は payload を `json.Decoder(UseNumber)` でデコード→`json.Marshal` で再
  エンコードして正規化する（空白除去・map キーの辞書順ソート・数値を json.Number で保持・
  重複キーは last-wins・末尾余剰データはエラー）。契約テストはこの**正規化後の同値**を
  冪等とみなし（空白差・キー順差は同一）、正規化後に異なる内容は競合とみなす。memfake も
  同じ正規化規則に従い、Adapter と観測挙動を一致させる（memfake だけが異なる判定をしない）。
- **時刻精度**：`StoredAt` が UTC で返り、Save→Get→Search 間で同一値（単調・丸めなし）で
  一貫する。精度要件は「保存時に付与した値が欠落・改変なく返る」こと。
- 同一 ID 同一内容の **冪等**：2 回目の Save が新規作成せず、1 回目と同一の観測結果
  （同一 `StoredAt`、記憶が重複しない）。
- 同一 ID 異内容で **`ErrConflict`**、かつ**元の記憶が保持される**（上書きされない）。
- 必須欠落（ID/Kind/Source/Origin/ObservedAt/Content のいずれか）/ 非 JSON / >1 MiB の
  各不正入力エラー（`ErrInvalidMemory` / `ErrContentTooLarge`）。
- Get 不在で `ErrNotFound`。
- Search：Source/Kind/時間範囲フィルタ、Limit=0→100、>1000→1000、<0→`ErrInvalidQuery`、
  一致なし→空スライス。
- **同一時刻の順序**：同一 `StoredAt` の複数記憶が、ID 降順の決定的順序で返る
  （実行間で安定）。
- **キャンセル時の副作用**：少なくとも**呼び出し前にキャンセル済みの ctx** で Save した
  場合、エラーを返し、かつその後の Get で `ErrNotFound`（記憶が残らない）。Get/Search も
  同様にエラー。実行中（処理途中）のキャンセルにおける部分書き込みの有無は Backend 挙動に
  依存するため、契約テストの保証対象は「呼び出し前キャンセル」に限定する。MILLEFEUILLE は
  append を単一トランザクションで行い、キャンセル時に部分行を残さない挙動が既存テスト
  （`TestOpen_NoPartialRowOnCancel`）で確認済みであり、Adapter はこの挙動に委ねる。memfake は
  各メソッド冒頭で `ctx.Err()` を確認し、書き込み前に中断する。
- `Close()` の冪等性（複数回呼び出しが安全）。

`memfake` と `millefeuilleadapter`（`t.TempDir()` の実ファイル SQLite）の双方へ適用し、
両者が同一の**観測挙動**を満たすことを保証する（R5 AC-2）。ハッシュアルゴリズムの一致は
要求しない（OQ-B）。

## Data flow

```
NAVETTE / Imagorium
   │  unefeuille.Store（契約のみに依存）
   ▼
unefeuille.Store 実装
   ├── millefeuilleadapter ──► millefeuille.Store ──► SQLite file
   └── memfake            ──► in-memory map（テスト/統合）
```

## API and data-model changes

- 新規追加のみ。`unefeuille` パッケージとサブパッケージを追加する。
- MILLEFEUILLE ルートパッケージ・`internal/*`・migrations・公開 API は無変更（COMP-001）。
- go.mod は変更不要（追加依存なし, NFR-001）。

## Error handling

- Adapter は Backend sentinel を `unefeuille` sentinel に `%w` で包んで変換（上表）。
- 入力検証は Save の冒頭で実施。memfake と Adapter 双方が同じ検証規則
  （必須: ID/Kind/Source/Origin/SchemaVersion/ObservedAt/Content、JSON valid、≤1 MiB）を適用。
  Adapter は MILLEFEUILLE 側の検証にも委ねるが、SchemaVersion/Origin 欠落は Adapter 到達前に
  `ErrInvalidMemory` を返し、二重でも挙動が一致するよう sentinel を揃える。
- ctx キャンセル：各メソッドは Backend の ctx 伝播に委ね、memfake は明示チェック。

## Security considerations

- SEC-001：Une Feuille は認証・認可を提供しない。`Labels.Visibility` は verbatim 保存・
  返却し、Search のフィルタ条件に用いない。アクセス制御は利用側境界の責務。
- SEC-002：`Labels.TrustLevel` を自動昇格・推論しない。既定 "unverified" を踏襲。
- trusted-caller モデルを README/doc.go に明記する。

## Compatibility and migration

- COMP-001：既存 `millefeuille` 公開 API を変更しない。Une Feuille は純粋な新規追加。
- 既存テスト（`millefeuille` ルート, internal/*）は不変で green を維持。
- マイグレーション不要（スキーマ変更なし）。

## Observability

- `SaveReceipt`（`ID` / `StoredAt`）とエラー文脈で冪等確認・監査に利用可能。Backend 固有の
  内容ハッシュは公開契約に含めない（OQ-A）。
- エラーは `%w` で原因を保持し、`errors.Is`/`errors.Unwrap` で診断可能。

## Test strategy

- 契約テスト（`contracttest.Run`）を memfake と millefeuilleadapter の双方へ適用。
- Adapter 固有テスト：MILLEFEUILLE sentinel → unefeuille sentinel の変換を個別検証。
- `go build ./...` / `go test ./...` / `go vet ./...`（NFR-003）。
- 既存 MILLEFEUILLE テストの非回帰を確認。

## NAVETTE からの利用例

```go
import (
    "github.com/habuka036/millefeuille/unefeuille"
    "github.com/habuka036/millefeuille/unefeuille/millefeuilleadapter"
    "github.com/habuka036/millefeuille/unefeuille/memfake"
)

// 本番: MILLEFEUILLE Backend
mem, err := millefeuilleadapter.Open(ctx, "/var/lib/navette/memory.db")
// 開発/テスト: fake（NAVETTE は Store 契約のみに依存）
mem = memfake.New()

// NAVETTE は unefeuille.Store 契約だけを握る。Backend 差し替えは注入で完結。
rec, err := mem.Save(ctx, unefeuille.Memory{
    ID: taskRunID, Kind: "navette.run.log", Source: "navette",
    Origin: "agent", ObservedAt: time.Now().UTC(),
    Content: json.RawMessage(`{"stage":"plan","status":"ok"}`),
})
```

NAVETTE は Une Feuille が無くても動く設計のため、`unefeuille.Store` は optional な
依存として扱い、未設定時は記憶連携をスキップする（NAVETTE design で詳細化）。

## Alternatives considered

- A: MILLEFEUILLE 型を alias で公開 → RD-1 で却下（Backend 非依存にならない）。
- B: Search に semantic 検索の口を用意 → Non-goal。MILLEFEUILLE 非対応のため却下。
- C: unefeuille が millefeuille を直接 import → 依存方向が崩れ交換性を損なうため却下
  （adapter サブパッケージに隔離）。

## Risks and mitigations

- R-1: memfake と Adapter で内容同一性判定（内部フィンガープリント）が異なり、冪等/競合の
  観測挙動がずれる → 契約テストで両実装の冪等・競合挙動を同一ケースで検証し吸収。判定の
  内部実装は各実装に閉じ、公開契約は「同一 ID+内容=冪等, 異内容=競合」の観測挙動のみ
  （OQ-B 確定）。
- R-2: 中立名と MILLEFEUILLE 名の対応ミス → Adapter の写像表を単体テストで固定（§フィールド
  保持表を基準）。
- R-3: Memory に将来フィールドが必要になる → 追加は後方互換な struct フィールド追加で対応
  （既存契約を壊さない）。
- R-4: requirements.md と design の必須フィールド/ NFR 範囲の文言差 → Tasks でドキュメント
  整合タスクを設け、Reconciliation で確認。

## Resolved review items (2026-10-14)

- OQ-A: 確定。`SaveReceipt` から Backend 固有の `ContentHash` を除外。MVP の公開契約は
  `ID` / `StoredAt` のみ。利用側は Backend 固有ハッシュに依存しない。
- OQ-B: 確定（レビュー補足反映）。memfake と Adapter で内容同一性の**ハッシュ算法**一致は
  不要。ただし payload の**正規化規則**は MILLEFEUILLE（UseNumber デコード→Marshal 再
  エンコード）に揃え、冪等・競合の観測挙動を契約テストで一致させる（memfake だけが異なる
  判定をしない）。
- DeviceID: `Memory.Device` として保持対象に追加（§フィールド保持表）。公開契約から
  対象外は MILLEFEUILLE 内部 `content_hash` のみ。
- Origin 必須: MILLEFEUILLE `validateInput` が `provenance.origin` を必須とするため、Une
  Feuille でも Save の必須フィールドとし Adapter 到達前に検証（§Origin）。
- Store 所有権 / Close: `Open` 由来は所有し Close する、`New(st)` は借用し Close しない、
  memfake は no-op。Close は冪等（§Store の所有権と Close）。
- Close × ctx: `Close()` は ctx を取らない。NFR-002 の適用範囲は Save/Get/Search に限定
  （§Close と Context の NFR 整合）。
- 契約テスト: 外部観測可能な振る舞い（時刻精度・同一時刻順序・JSON 同一性・キャンセル時の
  副作用なし）を検証し、内部実装に依存しない（§contracttest）。

## Open questions

- （なし。レビュー指摘はすべて上記 Resolved review items に反映済み。）
