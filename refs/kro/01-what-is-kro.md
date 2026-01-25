# 01. kro とは

## kro の位置付け

**kro (Kube Resource Orchestrator)** は、Kubernetes 上で「複数のリソースをまとめて 1 つの API として提供する」ための Kubernetes ネイティブなコントローラです。

Platform/SRE/セキュリティチームが、組織の標準(セキュリティ/監査/運用要件)を織り込んだ **カスタム API** を定義し、アプリ開発者はその API を `kubectl apply` 等の通常の Kubernetes ワークフローで利用します。

ポイントは「テンプレートエンジン」ではなく「Kubernetes API を増やす」ことです。

- Helm/Kustomize: 生成物(マニフェスト)を作る
- kro: クラスタ内に新しい CRD を生成し、インスタンスを reconcile して目的のリソース群を管理する

## 何が嬉しいか(価値)

- 複数リソースを 1 つの単位(インスタンス)として扱える
- 依存関係(順序)を kro が自動推論して作成/更新/削除する
- **CEL** による「値の受け渡し」「条件分岐」「ready 判定」を安全に記述できる
- RGD 作成時に **静的解析** (型/フィールド存在/依存循環など)を実施し、ミスを早期に検出できる
- 任意の CRD を含められる(ベンダロックインしない)。例: ACK, cert-manager, external-dns など

## kro の基本用語

### ResourceGraphDefinition (RGD)

`apiVersion: kro.run/v1alpha1` / `kind: ResourceGraphDefinition`

- kro のコア CRD
- クラスタスコープ
- 1 つの RGD が「新しいカスタム API(= 生成される CRD)」を定義する

RGD で定義すること:

- `spec.schema`: 生成される CRD(= 利用者が作るインスタンス)の schema
  - `spec` フィールド(入力)
  - `status` フィールド(出力)
  - 型定義(SimpleSchema)
- `spec.resources`: インスタンス作成時に生成/管理するリソース群
  - リソース間参照(CEL)で依存関係を表現
  - `includeWhen` (条件付き作成)
  - `readyWhen` (ready 条件)
  - `externalRef` (既存リソース参照)

### (生成される) CRD と Instance

RGD を apply すると kro は以下を行います:

1. RGD を静的解析し、問題がなければ **新しい CRD** を生成して API Server に登録
2. その CRD のインスタンス(= 開発者が作るカスタムリソース)を watch/reconcile する

この「インスタンス」が、裏側の複数リソース群の **single source of truth** になります。

例:

- RGD の `schema.kind: Application` を定義
- `kubectl apply` により `applications.kro.run` (例) の CRD が生成される
- 開発者は `kind: Application` のインスタンスを作るだけで、Deployment/Service/Ingress 等が一緒に管理される

### Resource Graph (DAG)

RGD 内の `resources` は、依存関係を持つ **有向非巡回グラフ(DAG)** として扱われます。

- ノード: 個々のリソース
- エッジ: CEL 式で参照した関係(参照先 -> 参照元)

依存関係は手で書かず、参照から kro が推論します。

## 全体の流れ(最小イメージ)

1) Platform が RGD を作成

```yaml
apiVersion: kro.run/v1alpha1
kind: ResourceGraphDefinition
metadata:
  name: my-application
spec:
  schema:
    apiVersion: v1alpha1
    kind: Application
    spec:
      name: string
      image: string | default="nginx"
  resources:
    - id: deployment
      template:
        apiVersion: apps/v1
        kind: Deployment
        metadata:
          name: ${schema.spec.name}
        spec:
          replicas: 1
          selector:
            matchLabels:
              app: ${schema.spec.name}
          template:
            metadata:
              labels:
                app: ${schema.spec.name}
            spec:
              containers:
                - name: app
                  image: ${schema.spec.image}
```

2) 開発者が生成された API のインスタンスを作成

```yaml
apiVersion: kro.run/v1alpha1
kind: Application
metadata:
  name: my-app
spec:
  name: my-app
  image: nginx:1.27
```

3) kro が Deployment などの underlying resources を作成/更新/削除し、status を集約

## kro のアーキテクチャ概観

kro コントローラには大きく 2 つの責務があります。

### 1) RGD Reconciler

- `ResourceGraphDefinition` を watch
- 静的解析(スキーマ、CEL、依存、循環、Kubernetes OpenAPI schema の整合性)を実施
- 生成 CRD を作成/更新
- Instance を扱うためのハンドラを登録

### 2) Dynamic Controller (インスタンス管理)

- RGD により生成された複数の CRD/GVR を動的に扱うための仕組み
- RGD 登録に応じて informer/handler を on-demand に構成し、インスタンスとその子リソースのイベントを 1 つのキューで処理する
- 子リソースの変更/削除も検知し、ドリフトを自動修復する

実装上は「RGD 毎に独立した Deployment を立てる」よりも軽量に、kro Pod 内で動的に watch が増えるイメージです(公式FAQにある “microcontroller” はこのハンドラの概念として捉えると理解しやすいです)。

## kro が “安全” と言われる理由

### CEL による式評価

CEL は副作用がなく、ループ/再帰もないため、ユーザーが書いた式を安全に実行できます。

### 静的解析(型/フィールド/循環)が RGD 作成時に走る

kro は RGD 作成時に Kubernetes API Server から OpenAPI schema を取得し、テンプレートと式を検証します。

- 存在しないフィールド参照: RGD 作成時に弾く
- 型不一致: RGD 作成時に弾く
- 循環依存: RGD 作成時に弾く

つまり「利用者がインスタンスを作って初めて壊れる」より前に、Platform チームの段階で検出できます。

## まず覚えるべき kro の設計原則

- RGD は “API を設計する” もの
  - instance spec は “入力”、instance status は “出力”
  - 出力は下位リソースの `status` 等から CEL で射影し、上位 API を安定させる
- 依存関係は “参照で表現する”
  - 参照した瞬間に依存が生まれ、順序が推論される
- 非同期で値が揃うもの(ロードバランサのIP、DBエンドポイント等)は `readyWhen` を活用して安全に待つ
- 可変構造(ConfigMap/Secret の `data` など)は `?` を使い、必要なら `.orValue()` でデフォルトを定義する
