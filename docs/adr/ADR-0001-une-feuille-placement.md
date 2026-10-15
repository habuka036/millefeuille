# ADR-0001: Une Feuille を MILLEFEUILLE リポジトリ内の Go パッケージとして配置する

- Status: Accepted
- Date: 2026-10-14

## Context

Imagorium の開発速度向上のため、NAVETTE や Imagorium 各コンポーネントが特定の
Memory Backend に強く依存せず、共通 API から記憶を利用できる抽象化レイヤ
「Une Feuille」が必要になった。

MILLEFEUILLE は v0.1.0 をリリース済みで、SQLite ベースの append-only な
observation store を提供する独立した再利用可能ライブラリである。Une Feuille は
この MILLEFEUILLE を主要な Backend として利用する。

Une Feuille を独立リポジトリにするか、既存リポジトリ内に配置するかを決める必要が
あった。独立リポジトリ化は version 管理・公開の自由度が高い一方、まだ API が
固まっていない初期段階ではリポジトリ分割・CI・module 公開の運用コストが先行する。

## Decision

Une Feuille は当面、MILLEFEUILLE リポジトリ内の Go パッケージ
（`github.com/habuka036/millefeuille/unefeuille`）として実装する。独立リポジトリには
しない。

## Rationale

- MILLEFEUILLE の既存実装（公開 `Store` インターフェース、ドメイン型、冪等・
  append-only の意味論）を最大限再利用でき、Adapter 実装が同一 module 内で完結する。
- API が未確定な初期段階では、リポジトリ分割に伴う module 公開・version 整合・CI
  の運用コストを回避できる。
- 代替案 (A) 独立リポジトリ化：将来 Backend が複数化し Une Feuille の API が安定した
  段階で再検討する余地を残す。現時点では時期尚早。
- 代替案 (B) NAVETTE 内に配置：Une Feuille は NAVETTE 専用ではなく Imagorium からも
  利用される汎用抽象のため、記憶基盤である MILLEFEUILLE 側に置く方が責務上自然。

## Consequences

- 利点：既存コードの再利用が容易。Adapter とドメイン型が同一 module で型整合する。
- 制約：Une Feuille の公開 API は MILLEFEUILLE の module version に紐づく。Une Feuille
  の破壊的変更は MILLEFEUILLE の version に影響する。
- 将来の影響：Backend が複数化し API が安定したら、独立 module 切り出しを ADR で
  再検討する（本 ADR を Superseded にしうる）。
