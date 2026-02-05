# kro v0.8.x を踏まえた `docs/PRD.md` 更新案

- 作成日: 2026-01-29
- 入力:
  - `docs/PRD.md`
  - `docs/update-kro-0.8.0.md`
  - 実装: `internal/controller/kany8scontrolplane_controller.go`
  - サンプル: `examples/kro/eks/eks-addons-rgd.yaml`, `examples/kro/eks/pod-identity-set-rgd.yaml`, `examples/kro/eks/platform-cluster-rgd.yaml`
  - 参考: `docs/kro/kro-0.8.0.md`, `docs/kro/kro-collections.md`, `docs/rgd-contract.md`, `docs/rgd-guidelines.md`, `docs/runbooks/kind-kro.md`

## 1. 結論

- kro v0.8.0 の Collections (`forEach`) は、`docs/PRD.md` の「部品 RGD + chaining」方針と相性が良い。
- Kany8s の controller は kro instance の **正規化 status**（`status.ready/endpoint/reason/message` と `status.kubeconfigSecretRef`）しか参照しないため、Collections 導入の主戦場は **RGD の書き方/サンプル/運用**であり、controller 側の設計は基本的に変えずに済む。
- 一方で Collections は「反復の省略」だけでなく、**依存関係の粒度（collection 単位の待ち）**と **削除（配列から外すと prune される）**の意味を持つため、PRD の技術制約・運用方針に注意点を追記しないと事故りやすい。

## 2. 現状整理（PRD と実装の境界）

### 2.1 Kany8s が kro に要求するインターフェース

- controller 実装（`internal/controller/kany8scontrolplane_controller.go`）は、RGD が生成する instance の `status` から以下だけを読む。
  - `status.ready: bool`
  - `status.endpoint: string`
  - `status.reason/message: string`（任意）
  - `status.kubeconfigSecretRef`（任意。存在する場合は kubeconfig Secret の reconcile も Ready 判定に含める）
- 上記の契約は `docs/rgd-contract.md` に整理済み。

### 2.2 kro v0.7.1 前提の記述が PRD に残っている

- `docs/PRD.md` の 11.2 は kro v0.7.1 の検証結果ベース。
- `docs/runbooks/kind-kro.md` も `KRO_VERSION=0.7.1`。
- kro v0.8.0 では Collections と「RGD schema の破壊的変更検知」が入るため、PRD の “技術的要求/運用制約” に差分を取り込む価値が高い。

## 3. PRD 更新方針（選択肢）

### Option A（推奨）: kro 前提を v0.8.x に更新し、v0.7.1 の注意点は “再検証が済むまで暫定で残す”

- PRD の 11.2 を「v0.7.1 の既知罠」+「v0.8.x の追加要素（Collections / schema 破壊検知）」の二段に整理する。
- サンプル/ドキュメント（特に UC4 の周辺リソース）を Collections 前提の “配列入力” に寄せる。

理由:

- Kany8s controller は kro の内実よりも status 契約に依存しているため、kro の機能追加（Collections）の恩恵を RGD 側で取り込みやすい。
- 一方で v0.7.1 の落とし穴（status materialization 等）が v0.8.x で解消されたかは未確定なので、既存の注意点を「削除」ではなく「再検証待ち」にするのが安全。

### Option B: PRD は v0.7.1 のまま、Collections は “将来/非MVP” の参考として追記

- 影響を最小化できるが、RGD サンプルが固定個数の手書きのままになり、UC4 の方針が具体化しにくい。

### Option C: 周辺 RGD の標準形を “Collections 必須” として一気に統一

- サンプルの方向性は揃うが、削除/スケールダウンや依存の粒度（全 item 待ち）の設計判断が一気に増える。

## 4. `docs/PRD.md` への具体追記案（差分イメージ）

以下は PRD を「どこに」「何を書くか」の案。実際の編集は別 PR/コミットで行う想定。

### 4.1 3. 製品原則

現行の「小さく分割・合成」を、次のニュアンスで補強する。

- “部品 RGD を増やす” と “同種リソースを増やす” を分離する。
  - 部品 RGD の内部では `forEach`（Collections）で反復を表現し、固定個数のリソース定義のコピペ増殖を避ける。
- `Ready` の定義を守る（ControlPlane ready と周辺 ready の分離）は維持する。
  - Collections により周辺リソースを増やしやすくなるほど、ControlPlane と周辺を同一 RGD に詰め込みたくなるため、PRD 側で “分離が原則” を明確にする。

追記テキスト案（箇条書き差し替え例）:

```text
- 小さく分割・合成: "巨大な 1 枚 RGD" を避け、部品 RGD + chaining で再利用可能にする。部品 RGD の内部では kro v0.8.x Collections (forEach) を使い、同種リソースの反復を配列入力で宣言的に表現する。
```

### 4.2 6. ユースケース（UC4: 周辺リソースの合成）

UC4 の “実現手段” を Collections 前提で具体化する。

- ユーザー入力を「配列（list）」で受け、RGD で `forEach` により増減させる。
  - 例: `addons: []AddonSpec`, `podIdentities: []PodIdentitySpec` など
- 環境差分を “YAML 断片の差し替え” ではなく “配列差分” として表現できること（レビューしやすさ/追跡しやすさ）を PRD の価値提案に接続する。

PRD に載せるミニ例（概念。EKS Addon を例にした配列入力）:

```yaml
schema:
  spec:
    region: string | required=true
    clusterName: string | required=true
    addons: "[]AddonSpec | default=[{name:'coredns', addonVersion:'...'}]"
  types:
    AddonSpec:
      name: string | required=true
      addonVersion: string | required=true
      preserve: boolean | default=true

resources:
  - id: addons
    forEach:
      - addon: ${schema.spec.addons}
    readyWhen:
      - ${each.?status.?status.orValue('') == 'ACTIVE'}
    template:
      apiVersion: eks.services.k8s.aws/v1alpha1
      kind: Addon
      metadata:
        name: ${schema.spec.clusterName + '-' + addon.name}
        annotations:
          services.k8s.aws/region: ${schema.spec.region}
      spec:
        clusterName: ${schema.spec.clusterName}
        name: ${addon.name}
        addonVersion: ${addon.addonVersion}
        preserve: ${addon.preserve}
```

### 4.3 11. 技術的要求 → 11.2（kro 実装制約への適合）

11.2 は次の構成に再整理する（既存の v0.7.1 知見は保持しつつ、v0.8.x の追加要素を追記）。

#### 11.2.1 v0.7.1 検証由来の既知制約（現状維持 + “再検証待ち” と明記）

既存の項目（`schema.*` が status CEL で使えない、`readyWhen` scope、文字列テンプレート、定数 status 禁止、`NetworkPolicy` 等）を残しつつ、
「kro v0.8.x で解消されたかは未検証。検証したら更新する」と注記する。

#### 11.2.2 kro v0.8.x Collections（forEach）の運用注意

`docs/kro/kro-collections.md` を根拠として、PRD に最低限の “事故りやすい点” を追記する。

- `forEach` は配列を返す CEL 式で駆動する。map の反復は順序が不定なので、**ソート済み配列へ変換**してから使う。
- **命名衝突**が最も事故りやすい。
  - `metadata.name` は instance 名（`schema.metadata.name` など）+ 反復キー（例: `addon.name`）を必ず含める。
- `includeWhen` は “コレクション全体” に作用し、個別アイテムの除外には使えない（個別フィルタは `filter()`）。
- 空コレクションは Ready 扱い（待つべきものが無い）。期待する意味に合わせて `minItems` / `includeWhen` / schema default を設計する。
- 依存は “コレクション単位” になりやすい。
  - コレクション参照があると、参照した側は **全アイテムの ready** を待ってから進む（大きい配列で遅延要因になり得る）。
- `forEach` と `externalRef` は併用不可。
- スケールダウン（配列から削除）で **対応するリソースが削除される**。
  - 削除が危険な対象は、RGD の設計（`preserve`/deletion policy 等）と運用（break-glass）でガードする。

#### 11.2.3 kro v0.8.x: RGD schema の “破壊的変更検知” の影響

`docs/kro/kro-0.8.0.md` の “Breaking Schema Change Detection” を踏まえ、RGD を “提供 API” として運用する方針を PRD に追記する。

- kro は RGD schema の破壊的変更（フィールド削除、型変更、required 追加など）を検知すると更新をブロックする。
- RGD の変更方針（案）:
  - 既存フィールドは原則削除しない（deprecate → 新フィールド追加 → 移行 → 最終削除は別 RGD/別 kind で行う）。
  - required 追加は避け、optional + default で前方互換を保つ。
  - 破壊的変更が必要な場合は `kro.run/allow-breaking-changes: "true"` を付けるのではなく、まず “新しい RGD 名/Kind（= 新しい instance API）で提供する” を優先する（既存 instance の運用を壊しにくい）。

### 4.4 12. マイルストーン/成果物の追記（例）

- M2（RGD サンプル）に、Collections を使った UC4 サンプル（Addon 群など）を “配列入力” で追加する。
- kro v0.8.x を kind で再検証し、`docs/runbooks/kind-kro.md` と 11.2 の注記を更新する。

## 5. PRD 外だが、整合のために同時に更新したい箇所（推奨）

- `docs/rgd-guidelines.md`
  - v0.7.1 pitfall セクションに加えて、Collections の gotcha セクションを追加（PRD の 11.2.2 と内容を揃える）。
- `docs/runbooks/kind-kro.md`
  - `KRO_VERSION` を v0.8.x に更新し、必要なら v0.7.1 時代の “RBAC を緩める必要” の注記を再確認する。
- `examples/kro/eks/eks-addons-rgd.yaml` / `examples/kro/eks/pod-identity-set-rgd.yaml`
  - Collections 版を追加（既存版は残す or 置き換える）。
  - 特に `PodIdentitySet` は “Role と Association の紐付け” と “削除時の意味” が設計分岐になるため、PRD に運用方針（削除の扱い）を明記する。

## 6. DoD（この更新が完了した状態）

- `docs/PRD.md` に以下が反映されている。
  - 製品原則に「部品 RGD 内での forEach 活用（反復に強い分割・合成）」が明文化されている。
  - UC4 に「配列入力 + Collections」による実現手段と最小例が載っている。
  - 11.2 に「Collections の gotcha」と「RGD schema 破壊的変更検知に対する運用方針」が追記されている。
- `docs/runbooks/kind-kro.md` の kro バージョン前提と、PRD の 11.2 の注記が矛盾していない。
