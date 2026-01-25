# 03. ResourceGraphDefinition (RGD) の書き方

この章は「Platform チームが RGD を設計/実装する」ための実践ガイドです。

RGD は kro の “唯一の設定 API” であり、RGD を apply すると **新しい CRD** がクラスタに生成されます。

## RGD の全体構造

```yaml
apiVersion: kro.run/v1alpha1
kind: ResourceGraphDefinition
metadata:
  name: my-application            # RGD 自体の名前(クラスタスコープ)
spec:
  schema: {}                      # 生成される CRD の schema
  resources: []                   # インスタンスが管理するリソース群
status:                           # kro が管理
  state: Active|Inactive|...
  topologicalOrder: []
  conditions: []
  resources: []                   # 依存情報など
```

ポイント

- `metadata.name` は **RGD の名前**。生成される CRD の Kind/Group/Version とは別物です。
- 生成される CRD の識別子は `spec.schema.{group,apiVersion,kind}` で決まります。

## spec.schema: 生成される CRD の設計

`spec.schema` は、利用者(アプリ開発者)が作るインスタンスの shape を定義します。

### 必須フィールド

- `schema.apiVersion`: 生成 CRD のバージョン名(例: `v1alpha1`)
- `schema.kind`: 生成 CRD の Kind 名(例: `Application`)

任意フィールド

- `schema.group`: API group (省略時 `kro.run`)

例:

```yaml
schema:
  apiVersion: v1alpha1
  kind: Application
  group: mycompany.io
```

この場合、インスタンスの `apiVersion` は `mycompany.io/v1alpha1` になります。

### schema.spec: 入力(インスタンス spec)

利用者が指定できる入力を SimpleSchema で定義します。

```yaml
schema:
  spec:
    name: string | required=true
    replicas: integer | default=3 minimum=1 maximum=100
    image: string | default="nginx"
    ingress:
      enabled: boolean | default=false
      host: string
```

### schema.status: 出力(インスタンス status)

インスタンス status は「下位リソースの値を射影して、上位 API の状態として見せる」ために使います。

```yaml
schema:
  status:
    availableReplicas: ${deployment.status.availableReplicas}
    serviceIP: ${service.spec.clusterIP}
    endpoint: "http://${service.metadata.name}"  # 文字列テンプレート
```

仕様

- status は CEL 式から型を推論して、生成 CRD の OpenAPI schema を自動生成します。
- `conditions` と `state` は kro が全インスタンスに自動注入する予約語です(自分で定義しても上書きされます)。

### schema.types: カスタム型

複雑な spec を整理したい場合、再利用可能な型を `types` に定義できます。

```yaml
schema:
  types:
    ContainerConfig:
      image: string | required=true
      env: "map[string]string"
  spec:
    main: ContainerConfig
    sidecars: "[]ContainerConfig"
```

### schema.additionalPrinterColumns

`kubectl get <plural>` の表示カラムを制御できます。

```yaml
schema:
  additionalPrinterColumns:
    - name: Replicas
      type: integer
      jsonPath: .spec.replicas
    - name: Ready
      type: string
      jsonPath: .status.state
```

注意

- `additionalPrinterColumns` を明示すると「デフォルトのカラム」は自動追加されません。必要なら自分で全て書きます。

### schema の immutable 制約

`schema.apiVersion` / `schema.kind` / `schema.group` は immutable として扱われます。

- 既存 RGD の Kind/Group/Version を変えるには「別の RGD を新規作成」する方針を推奨します。
- 既存の生成 CRD とインスタンスがある状態で変えると、Kubernetes 的に別リソースになり移行が必要です。

## spec.resources: 下位リソースの定義

### Resource の基本形

各要素は少なくとも `id` を持ち、以下のどちらか一方を指定します。

- `template`: kro が作成/更新/削除するリソース
- `externalRef`: 既存リソースを参照するだけ(作成/更新/削除しない)

```yaml
resources:
  - id: deployment
    template: {}

  - id: platformConfig
    externalRef: {}
```

#### id 命名ルール

`id` は CEL の変数名として使われるため、**lowerCamelCase** を推奨します。

- OK: `deployment`, `webServer`, `dbPrimary`
- NG: `web-server` (CEL 上 `web - server` と解釈され得る)

### template: 通常の Kubernetes マニフェスト + CEL

template は「有効な Kubernetes YAML」です。値の一部に `${...}` の形で CEL を埋め込みます。

```yaml
- id: service
  template:
    apiVersion: v1
    kind: Service
    metadata:
      name: ${schema.spec.name}-svc
    spec:
      selector: ${deployment.spec.selector.matchLabels}
      ports:
        - port: 80
          targetPort: 80
```

### externalRef: 既存リソース参照

```yaml
- id: sharedConfig
  externalRef:
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: platform-config
      namespace: platform-system   # 省略時はインスタンス namespace
```

参照先が存在するまで、依存するリソースの reconcile は進みません。

### includeWhen: 条件付き作成

```yaml
- id: ingress
  includeWhen:
    - ${schema.spec.ingress.enabled}
  template: {}
```

仕様

- `includeWhen` は boolean を返す CEL 式の配列
- 全て true のときだけリソースを含める(AND)
- 現状 `schema.spec` のみ参照可能(リソース生成前に評価するため)
- includeWhen が false で skip されたリソースを参照している子リソースも skip される

### readyWhen: ready 条件

```yaml
- id: database
  template: {}
  readyWhen:
    - ${database.status.conditions.exists(c, c.type == "Ready" && c.status == "True")}
    - ${database.status.?endpoint != ""}
```

仕様

- `readyWhen` は boolean を返す CEL 式の配列
- 対象リソース自身のフィールドのみ参照可能(例では `database.*`)
- 条件を満たすまで、そのリソースに依存する子リソースの作成を待つ

`readyWhen` は「作成順序」ではなく「依存が先に進んでよいか(値が揃ったか)」の制御です。

## kro で参照できる変数

### schema

- `schema.spec`: インスタンス spec(利用者入力)
- `schema.metadata`: インスタンス metadata(name/namespace/labels/annotations/uid...)

例:

```yaml
metadata:
  name: ${schema.spec.name}
  namespace: ${schema.metadata.namespace}
  labels: ${schema.metadata.labels}
```

### 他リソース(id 変数)

`resources[].id` で定義した変数名で、別リソースの `spec/metadata/status` 等を参照できます。

```yaml
selector: ${deployment.spec.selector.matchLabels}
endpoint: ${service.status.loadBalancer.ingress[0].hostname}
```

この参照が依存関係になり、kro が自動で作成順序を推論します。

## 例: Web アプリ(Deployment + Service + Optional Ingress)

`includeWhen` と status 射影まで含めた例です。

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
      name: string | required=true
      image: string | default="nginx"
      replicas: integer | default=3 minimum=1
      ingress:
        enabled: boolean | default=false
    status:
      availableReplicas: ${deployment.status.availableReplicas}
      deploymentConditions: ${deployment.status.conditions}

  resources:
    - id: deployment
      template:
        apiVersion: apps/v1
        kind: Deployment
        metadata:
          name: ${schema.spec.name}
        spec:
          replicas: ${schema.spec.replicas}
          selector:
            matchLabels:
              app: ${schema.spec.name}
          template:
            metadata:
              labels:
                app: ${schema.spec.name}
            spec:
              containers:
                - name: ${schema.spec.name}
                  image: ${schema.spec.image}
                  ports:
                    - containerPort: 80

    - id: service
      template:
        apiVersion: v1
        kind: Service
        metadata:
          name: ${schema.spec.name}-svc
        spec:
          selector: ${deployment.spec.selector.matchLabels}
          ports:
            - protocol: TCP
              port: 80
              targetPort: 80

    - id: ingress
      includeWhen:
        - ${schema.spec.ingress.enabled}
      template:
        apiVersion: networking.k8s.io/v1
        kind: Ingress
        metadata:
          name: ${schema.spec.name}-ingress
        spec:
          rules:
            - http:
                paths:
                  - path: "/"
                    pathType: Prefix
                    backend:
                      service:
                        name: ${service.metadata.name}
                        port:
                          number: 80
```

## RGD の状態確認(作成時の検証結果)

RGD は作成時に静的解析され、通れば Active になります。

```bash
kubectl get rgd my-application -o wide
kubectl describe rgd my-application
kubectl get rgd my-application -o yaml
```

代表的な status.conditions (0.7.1):

- `ResourceGraphAccepted`: RGD のバリデーションが通ったか
- `KindReady`: 生成 CRD が登録されたか
- `ControllerReady`: インスタンスを watch できる状態か

RGD が Active にならない場合は、まず `ResourceGraphAccepted` の `message` を見て修正します。

## 補足: エディタ支援(Experimental)

公式ドキュメントには大きく出ていませんが、リポジトリには **kro Language Server(試験的)** が含まれています。

- パス: `tools/lsp/`
- VS Code 向けの簡易 extension と、LSP server が同梱されています

利用例(リポジトリを clone した前提):

```bash
cd tools/lsp
make install
make lsp
```

生成される server バイナリ例:

- `tools/lsp/server/kro-lsp`

注意

- まだ 0.0.x の試験的実装で、公式に安定サポートされているとは限りません。
- とはいえ「RGD のローカル検証/補完」を効かせたい場合の出発点にはなります。
