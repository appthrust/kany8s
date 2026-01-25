# 05. CEL 実践リファレンス(kro 用)

CEL(Common Expression Language)は、kro における「値の受け渡し」「条件」「ready 判定」「status 集約」の中心です。

## CEL の基本

### kro での式の書き方

CEL は `${` と `}` で囲みます。

```yaml
metadata:
  name: ${schema.spec.name}
```

### 2 種類の埋め込み

1) フィールド値が “式だけ” の場合(standalone)

```yaml
spec:
  replicas: ${schema.spec.replicas}  # integer を返す
```

- 式の戻り値がフィールド型と一致する必要があります。

2) 文字列テンプレート(複数式を文字列に埋め込む)

```yaml
data:
  url: "https://${service.metadata.name}.${service.metadata.namespace}.svc"
```

- 文字列テンプレート内の式は **全て string を返す必要** があります。
- 数値等は `string()` で変換してください。

```yaml
message: "replicas=${string(deployment.status.replicas)}"
```

## kro が提供する “変数”

### schema

インスタンス(利用者が作成した custom resource)自体を表します。

- `schema.spec`: 入力
- `schema.metadata`: name/namespace/labels/annotations/uid など
- `schema.apiVersion` / `schema.kind` なども参照可能

### resources の id 変数

RGD 内の `resources[].id` が CEL 上の変数になります。

```yaml
value: ${deployment.status.availableReplicas}
selector: ${deployment.spec.selector.matchLabels}
```

この参照はそのまま依存関係になり、kro が DAG を推論します。

## フィールド参照

- ドットでネストを辿る: `${deployment.spec.template.spec.containers[0].image}`
- 配列: `[index]`
- Map: `.key` (ただしキーが動的な場合は後述の `?` を使う)

## Optional operator `?`

`?` は「そのフィールドが無い場合は null を返す」アクセサです。

使いどころ

- ConfigMap/Secret の `data` のようにキーがスキーマで決まらない
- status が非同期で生える/生えない可能性がある

例:

```yaml
value: ${config.data.?DATABASE_URL}
```

### `.orValue()` でデフォルトを付ける

`?` の結果が null の場合にデフォルトを返します。

```yaml
value: ${config.data.?LOG_LEVEL.orValue("info")}
```

注意

- `?` を使うと「そのフィールドが存在するか」の静的検証が弱まります。
- 期待するキー/構造は RGD のドキュメントとして明示し、`.orValue()` で安全側に倒すのが運用しやすいです。

## `includeWhen` / `readyWhen` と CEL

### includeWhen

- boolean を返す必要
- 現状 `schema.spec` のみ参照可能

```yaml
includeWhen:
  - ${schema.spec.ingress.enabled}
  - ${schema.spec.environment == "production"}
```

### readyWhen

- boolean を返す必要
- 対象リソース自身のみ参照可能

```yaml
readyWhen:
  - ${deployment.status.availableReplicas > 0}
  - ${deployment.status.conditions.exists(c, c.type == "Available" && c.status == "True")}
```

## 型と型チェック

kro は RGD 作成時に Kubernetes OpenAPI schema を参照し、CEL 式を型チェックします。

### 代表的な型不一致例

```yaml
# replicas は integer を要求する
spec:
  replicas: ${schema.spec.name}  # string -> integer でエラー
```

### 構造的互換性(duck typing)

kro は単純な一致だけではなく、Kubernetes でよくある map/struct の互換を考慮します。

- map <-> struct の互換
- struct の subset(余計なフィールドを含まない) など

## オブジェクト/マップを式で作る `${{ ... }}`

CEL の map リテラルは `{...}` を使いますが、kro の `${...}` パーサと衝突しやすいので、kro では **`${{ ... }}`(二重波括弧)** が例として示されています。

```yaml
metadata:
  labels: ${{"app": schema.spec.name, "env": schema.spec.environment}}
```

また、配列要素としてオブジェクトを作る場合:

```yaml
containers:
  - ${{"name": "app", "image": schema.spec.image}}
```

注

- kro 側の実装/バージョンで扱いが変わる可能性があるため、複雑なオブジェクト生成は「可能なら素直に YAML で書く」「必要なところだけ式で埋める」を推奨します。

## よく使う式パターン

### 三項演算子

```yaml
image: ${schema.spec.env == "prod" ? "nginx:stable" : "nginx:latest"}
```

### 文字列フォーマット

```yaml
roleArn: ${"arn:aws:iam::%s:role/%s".format([schema.spec.accountId, schema.spec.roleName])}
```

### リスト処理(filter/map/all/exists/size)

```yaml
# 条件配列から Ready を探す
${deployment.status.conditions.exists(c, c.type == "Available" && c.status == "True")}

# filter
${deployment.status.conditions.filter(c, c.status == "True")}

# map
${pods.items.map(p, p.metadata.name)}

# size
${service.status.loadBalancer.ingress.size() > 0}
```

## kro で使える CEL ライブラリ(0.7.1)

公式ドキュメントで明示されているもの:

- Lists: `cel-go/ext` Lists
- Strings: `cel-go/ext` Strings
- Encoders: `cel-go/ext` Encoders
- Random: kro 独自(例: random)
- URLs: Kubernetes apiserver CEL library
- Regex: Kubernetes apiserver CEL library

例: base64 decode (Encoders)

```yaml
token: "${ string(base64.decode(string(secret.data.uri))) }/oauth/token"
```

## ベストプラクティス

- `?` は “最後の手段” として使い、可能なら schema を構造化して型安全にする
- 文字列テンプレートは `string()` を忘れずに
- 非同期で生える値(LoadBalancer IP/hostname、DB endpoint 等)は `readyWhen` で待つ
- 条件式で float の等価比較は避ける(丸め誤差で状態が揺れる)
