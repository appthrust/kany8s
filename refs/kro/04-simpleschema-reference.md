# 04. SimpleSchema 詳細リファレンス

SimpleSchema は、RGD の `spec.schema.spec` などで使う「人間が書きやすい OpenAPI 互換のスキーマ記述」です。

この章は `v0.7.1` の公式仕様(https://kro.run/api/specifications/simple-schema)をベースに、実務上の注意点を補足します。

## SimpleSchema を使う場所

- `spec.schema.spec`: 生成 CRD の `spec` スキーマ(入力)
- `spec.schema.types`: 生成 CRD のカスタム型定義
- `spec.schema.status`: 生成 CRD の `status` スキーマ(出力) ※こちらは CEL 式で値を定義し、型は式から推論される

## 型(Types)

### 基本型

- `string`
- `integer`
- `boolean`
- `float`

```yaml
spec:
  name: string
  replicas: integer
  enabled: boolean
  ratio: float
```

### 構造体(ネストした object)

フィールドをネストすれば、構造化されたオブジェクトになります。

```yaml
spec:
  ingress:
    enabled: boolean | default=false
    host: string
    path: string | default="/"
```

### 配列(Array)

`[]` 記法。

```yaml
spec:
  ports: "[]integer"
  tags: "[]string"
```

YAML の都合で、`[]` を含む型はクォートするのが安全です。

### Map

`map[keyType]valueType`。

```yaml
spec:
  labels: "map[string]string"
  weights: "map[string]integer"
```

### unstructured object (注意)

`object` は「任意の JSON object」を受け入れる型で、スキーマ検証を弱めます。

```yaml
spec:
  values: object | required=true
```

注意

- `object` を使うとフィールド単位の型チェックが効かなくなり、kro の静的解析メリットが小さくなります。
- 可能な限り構造化した型で表現するのが推奨です。

### custom types

`types` に型を定義し、`spec` から参照できます。

```yaml
schema:
  types:
    MyType:
      value1: string | required=true
      value2: integer | default=42
  spec:
    env: "map[string]MyType"
    sidecars: "[]MyType"
```

## マーカー(Markers)

フィールド定義の後ろに `|` 区切りで制約や説明を付けます。

```yaml
spec:
  name: string | required=true description="Application name"
  replicas: integer | default=3 minimum=1 maximum=10
  mode: string | enum="debug,info,warn,error" default="info"
```

### よく使うマーカー

- `required=true`
- `default=<value>`
- `description="..."`
- `enum="a,b,c"`
- `minimum=<n>` / `maximum=<n>`
- `immutable=true` (作成後に変更不可)

### 文字列向け

- `pattern="regex"`
- `minLength=<n>` / `maxLength=<n>`

```yaml
spec:
  email: string | pattern="^[\\w\\.-]+@[\\w\\.-]+\\.\\w+$" required=true
  countryCode: string | pattern="^[A-Z]{2}$" minLength=2 maxLength=2
```

### 配列向け

- `uniqueItems=true|false`
- `minItems=<n>` / `maxItems=<n>`

```yaml
spec:
  ports: '[]integer | uniqueItems=true minItems=1'
  tags: '[]string | uniqueItems=true minItems=1 maxItems=10'
```

## default 値の書き方(実務の落とし穴)

default は YAML の構文と衝突しやすいので注意します。

例:

```yaml
spec:
  image: string | default="nginx"
  ports: '[]integer | default=[80]'
  labels: 'map[string]string | default={"app":"demo"}'
```

ポイント

- `default=[80]` のような配列/オブジェクトの default は、全体をクォートするのが安全
- 文字列 default は `default="..."` が読みやすい

## status フィールド(式から型推論される)

`schema.status` は、下位リソースから CEL で値を取り出し、インスタンス status に反映します。

```yaml
schema:
  status:
    availableReplicas: ${deployment.status.availableReplicas}  # integer
    endpoint: ${service.status.loadBalancer.ingress[0].hostname} # string
    metadata: ${deployment.metadata}                             # object
```

### 単一式 vs 文字列テンプレート

フィールド値が「単一の式」だけの場合は、式の戻り値型で扱われます。

```yaml
replicas: ${deployment.status.replicas}   # integer
```

複数の式を文字列内に埋め込む場合は、**必ず文字列** になり、各式も文字列である必要があります。

```yaml
summary: "replicas=${string(deployment.status.replicas)} ready=${string(deployment.status.availableReplicas)}"
```

## kro が自動注入する status フィールド(予約語)

インスタンスには kro により以下が自動で追加/管理されます。

- `status.conditions` (階層的 condition)
- `status.state` (`ACTIVE|IN_PROGRESS|FAILED|DELETING|ERROR`)

これらは予約語のため、`schema.status` に同名フィールドを定義しても上書きされます。

## 浮動小数点(float)の注意

公式仕様でも注意喚起されている通り、`float`/`double` 相当を条件式(`includeWhen`/`readyWhen`)に使う場合、丸め誤差で状態が揺れて意図しない requeue が発生する可能性があります。

- 条件判定には可能なら `string`/`integer`/`boolean` を使う
- float の等価比較(==)は避ける
