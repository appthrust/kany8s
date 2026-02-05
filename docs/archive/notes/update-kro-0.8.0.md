# kro v0.8.0 Collections による PRD 拡充余地の調査

- 作成日: 2026-01-29
- 対象ドキュメント:
  - `docs/PRD.md`
  - `docs/kro/kro-0.8.0.md` (release notes)
  - `docs/kro/kro-collections.md` (Collections / forEach の仕様・gotcha)

## 1. 結論

`kro v0.8.0` の Collections (`forEach`) は、`docs/PRD.md` の「部品RGD + chaining」方針と相性が良く、特に **周辺リソース(Addon 群 / PodIdentity 等)の “繰り返し” を YAML 手書きで増殖させない** という形で、PRD の具体性を上げられる余地がある。

一方で、Collections は「便利なループ」ではなく **依存関係・ready の粒度・削除(スケールダウン)の意味**を変える機能でもあるため、PRD の技術制約・運用設計に “注意点” を追記しないと事故りやすい。

## 2. PRD で拡充できるポイント（追記候補）

### 2.1 製品原則: 「小さく分割・合成」を “繰り返しに強い” 方針として明文化

`docs/PRD.md` には「巨大な 1 枚 RGD を避け、部品 RGD + chaining で再利用可能にする」とある。
ここに「部品 RGD の内部では `forEach` を使い、同種リソースの反復を宣言的に表現する」旨を追記すると、RGD 作者が採るべき具体パターンが明確になる。

追記のニュアンス案:

- 部品 RGD は「固定個数の手書き」ではなく「配列入力(= spec) + `forEach`」で拡張しやすい形に寄せる。
- “部品を増やす” と “反復を増やす” を分離し、変更理由が分かる RGD にする。

### 2.2 ユースケース/スコープ: UC4(周辺リソース合成)を Collections 前提の実現手段として整理

`docs/PRD.md` の UC4（Addon/S3/SQS/EventBridge 等を部品RGDとして提供し、必要に応じて chaining）は、Collections により次を満たしやすくなる。

- ユーザーが `kroSpec` に配列で “欲しい個数” を渡し、RGD が動的に増減させる
  - 例: `addons: []AddonSpec`, `podIdentities: []PodIdentitySpec`, `buckets: []BucketSpec` など
- “環境差分” を `kroSpec` のリスト差分として表現できる（固定 YAML 断片の差し替えより追跡しやすい）

PRD の「Scope 外/Planned」や「M2/M4/M5 のドキュメント/サンプル計画」に、Collections を使った “リスト入力のサンプル” を明記すると、将来のカタログ拡張の方向性が揃う。

### 2.3 技術的要求(11.2): kro v0.8.0 Collections の制約・gotcha を追加

`docs/PRD.md` の 11.2 は kro v0.7.1 検証に基づく制約が中心なので、v0.8.0 の Collections について最低限の運用注意を追記できる。

追記候補（要点のみ）:

- `forEach` は配列を返す CEL 式で駆動する。map 反復は順序が不定なので **ソート済み配列へ変換**してから使う。
- **名前衝突**が最も事故りやすい。`metadata.name` に instance 名 + 反復キーを必ず含める。
- `includeWhen` は “コレクション全体” に作用し、個別アイテムの除外には使えない（個別フィルタは `filter()`）。
- 空コレクションは Ready 扱い（= 待つべきものが無い）。期待する意味に合わせて `minItems` や `includeWhen` を使う。
- 依存関係は “コレクション単位” で張られやすい（参照した側は **全アイテムの ready** を待つ）。
- `forEach` と `externalRef` は併用不可。
- スケールダウン（配列から削除）で **対応するリソースが削除される**。削除が危険な対象は設計でガードする。

## 3. 現行の参照 RGD 例に当てた場合の適用候補

### 3.1 `examples/kro/eks/eks-addons-rgd.yaml`: 固定 3 個の Addon を Collections 化

現状は `coredns` / `kubeProxy` / `vpcCni` を個別 resource として手書きしている。
Collections を使うと「Addon の配列入力」+「単一テンプレート」で表現でき、追加/削除が YAML の反復コピペではなくリスト編集になる。

イメージ（概念スケッチ）:

```yaml
schema:
  spec:
    region: string | required=true
    clusterName: string | required=true
    addons: '[]AddonSpec | default=[{name:"coredns", addonVersion:"..."}, ...]'
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
      - ${each.?status.?status.orValue("") == "ACTIVE"}
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

PRD への含意:

- “周辺リソース RGD” をカタログとして増やすとき、固定個数のサンプルではなく「配列で増減できる」形を標準にできる。

### 3.2 `examples/kro/eks/pod-identity-set-rgd.yaml`: Role + Association のペアを Collections 化

現状は 2 ペア（ExternalDNS / ALB Controller）が手書きで、追加するとコピペ増殖になる。
Collections を使うと「PodIdentity の配列入力」を単一テンプレートへ落とせる。

ただし “Role と Association の紐付け” をどう表現するかで設計が分岐する。

- 案A（推奨）: 1つの spec 配列を index で共有し、Role コレクションと Association コレクションを 2 本立てにする
  - 参照は `idx` を使って `roles[idx]` の ARN を引く
  - 参照により依存はコレクション単位になり得る点に注意（全 Role を待ってから Association を作り始める）
- 案B: Association 側が Role コレクションを `filter()` して該当 Role を引く
  - 名前/ラベルの設計が肝、誤ると衝突・誤参照を起こす

PRD への含意:

- 「周辺リソースの合成」を“手書き blocks を増やす”から“入力配列を増やす”へ寄せられる。
- 一方で「削除（配列から外す）」が実リソース削除に直結するため、運用/ガード（break-glass 方針）が必要。

## 4. Kany8s の設計原則との整合（PRD へ書けるメッセージ）

- provider-agnostic controller という境界は維持される
  - Kany8s が読むのは引き続き `kro instance.status` の正規化値のみ
  - “複数リソースをどう作るか” は RGD 側に閉じ込められる
- 「Secrets は最小限」「outputs をコアにしない」方針と衝突しない
  - Collections は “同種リソースの個数管理” を強化するだけで、outputs 戦略は別論点
- 「ControlPlane Ready の定義を守る」方針と両立
  - Addons 等は ControlPlane RGD から分離したままでも、Collections はその部品 RGD 内で使える

## 5. 次のアクション（PRD 拡充のための作業案）

- PRD の 11.2 に「Collections の gotcha（命名/順序/空コレクション/削除/依存）」を追記する。
- PRD の UC4（周辺リソース合成）に「配列入力 + Collections で増減できる」具体例（Addon 群など）を 1 つ追加する。
- サンプル RGD の方向性を揃えるため、`examples/kro/eks/eks-addons-rgd.yaml` を “Collections を使う版” に置き換える/併記する（検証は kind + kro v0.8.x で実施）。
