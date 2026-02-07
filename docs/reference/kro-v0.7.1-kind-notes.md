# kro 調査メモ (kind + kro v0.7.1)

このファイルは、`refs/kro/examples/` を **kro v0.7.1** 実装に当てて検証した結果(仕様/挙動/落とし穴)をまとめたメモです。

目的:

- `refs/kro/examples/` が kro の実装仕様を満たしているかを確認
- kind 上で kro をインストールし、動作確認を実施

---

## 検証環境

- kind: v0.31.0
- Kubernetes: v1.35.0 (kindest/node:v1.35.0)
- kro: v0.7.1 (`kro-core-install-manifests.yaml`)
- context: `kind-kro-examples`

補足:

- 事前に `kubectl` / `kind` / `helm` が入っていない環境だったため、`curl` で `kubectl` と `kind` を導入して検証しました。
- kro のインストールは Helm ではなく raw manifests (GitHub Releases) を利用しました。

---

## 重要な発見 (docs と実装の差分 / 実装制約)

### 1) `spec.schema.status` の CEL 環境には `schema` が無い

kro v0.7.1 では `spec.schema.status` 内の CEL で `schema.*` を参照すると、以下のように **RGD が reject** されます。

```
undeclared reference to 'schema'
```

確認したこと:

- `${schema.spec.*}` / `${schema.metadata.*}` は `spec.schema.status` では使えない
- `${spec.*}` や `${metadata.*}` のような変数も存在しない
- `spec.schema.status` で使えるのは、基本的に **resource id 変数** (例: `${deployment.status.availableReplicas}`) 側

影響:

- `refs/kro/examples/` の RGD は当初すべて `spec.schema.status` で `schema.*` を参照していたため、そのままだと全例が `ResourceGraphAccepted=False` になりました。

回避策(現状の実装前提):

- status に spec/metadata 相当の値を出したい場合は、
  - (a) そもそも status に出さない / spec をそのまま見せる
  - (b) いったんどこかの managed resource (Deployment/Service 等) のフィールドに反映し、そこから status を射影する

### 2) `readyWhen` は self resource しか参照できない (docs 通り)

`readyWhen` は **対象 resource 自身のみ参照可能**です。

- OK: `${deployment.status.availableReplicas > 0}`
- NG: `${schema.spec.replicas > 0}` / `${service.status...}`

`refs/kro/examples/04-rgd-chaining-fullstack/02-webapp-rgd.yaml` などで `readyWhen` が `schema.*` を参照しており、修正が必要でした。

### 3) `spec.schema.status` の「文字列テンプレート」は安全ではない

`spec.schema.status` で次のような記法を使うと:

```yaml
status:
  endpoint: "http://${service.metadata.name}"
```

実測では `http://` のリテラルが落ち、`${...}` の評価結果だけが残るケースがありました。

安全策:

- status の string は **CEL 1式で連結して返す**

```yaml
status:
  endpoint: ${"http://" + service.metadata.name}
```

### 4) optional resource (`includeWhen=false`) を status 式で参照すると field 自体が欠落しやすい

`includeWhen` で skip された resource を status 式で参照すると、optional chaining (`?`) と `.orValue("")` を使っても
status field がそもそも出力されない(欠落する)ケースがありました。

設計の実務メモ:

- 「常に出したい status」は optional resource に依存させない
- optional な status は「出たり出なかったり」を許容する or 別の設計にする

### 5) `spec.schema.status` の各 field は「何かしら resource を参照」する必要がある

`spec.schema.status` に定数を置く(例: `ok: ${true}`)と、RGD が reject されました。

```
instance status field must refer to a resource: status.ok
```

このため kro v0.7.1 の実装前提では:

- status は **必ず resource id 変数** (`${deployment...}` など) を含める必要がある
- 「spec をそのまま echo する」「常に一定の status を出す」も直接はできない

### 6) `NetworkPolicy` を含む graph は v0.7.1 で Ready にならない (readyWhen でも救えない)

kind での実測では、RGD に `NetworkPolicy` を含めると、
下位リソースが作成されても instance が `IN_PROGRESS` から進まないケースを再現しました。

確認したこと:

- `Namespace + NetworkPolicy` を含むと `ResourcesReady=Unknown(ResourcesInProgress)` で固定
  - `Namespace + NetworkPolicy` だけでも再現
  - `Namespace + ResourceQuota + NetworkPolicy` でも再現
- `readyWhen: [${true}]` を付けても解消しない

対照(OK):

```bash
# Namespace + ResourceQuota
kubectl apply -f - <<'EOF'
apiVersion: kro.run/v1alpha1
kind: ResourceGraphDefinition
metadata:
  name: test-rq-only.kro.run
spec:
  schema:
    apiVersion: v1alpha1
    kind: TestRqOnly
    spec:
      name: string | required=true
  resources:
    - id: ns
      template:
        apiVersion: v1
        kind: Namespace
        metadata:
          name: ${schema.spec.name}
    - id: rq
      template:
        apiVersion: v1
        kind: ResourceQuota
        metadata:
          name: demo
          namespace: ${ns.metadata.name}
        spec:
          hard:
            requests.cpu: "1"
            requests.memory: 1Gi
EOF

kubectl apply -f - <<'EOF'
apiVersion: kro.run/v1alpha1
kind: TestRqOnly
metadata:
  name: trq-only-1
spec:
  name: trq-only-1
EOF

kubectl get testrqonly trq-only-1 -o wide
kubectl describe testrqonly trq-only-1

kubectl get ns trq-only-1 -o wide
kubectl -n trq-only-1 get resourcequota demo -o wide
```

最小再現(NG):

```bash
# Namespace + NetworkPolicy
kubectl apply -f - <<'EOF'
apiVersion: kro.run/v1alpha1
kind: ResourceGraphDefinition
metadata:
  name: test-np-only.kro.run
spec:
  schema:
    apiVersion: v1alpha1
    kind: TestNpOnly
    spec:
      name: string | required=true
  resources:
    - id: ns
      template:
        apiVersion: v1
        kind: Namespace
        metadata:
          name: ${schema.spec.name}
    - id: np
      template:
        apiVersion: networking.k8s.io/v1
        kind: NetworkPolicy
        metadata:
          name: demo
          namespace: ${ns.metadata.name}
        spec:
          podSelector:
            matchLabels: {}
          policyTypes:
            - Ingress
          ingress:
            - from:
                - podSelector: {}
EOF

kubectl apply -f - <<'EOF'
apiVersion: kro.run/v1alpha1
kind: TestNpOnly
metadata:
  name: tnp-only-1
spec:
  name: tnp-only-1
EOF

kubectl get testnponly tnp-only-1 -o wide
kubectl describe testnponly tnp-only-1

kubectl get ns tnp-only-1 -o wide
kubectl -n tnp-only-1 get networkpolicy demo -o wide
```

推測(強め):

- v0.7.1 の readiness 判定が **kstatus の Unknown を許容しない** 実装になっており、
  `NetworkPolicy` が未対応(Unknown 扱い)で詰まっている可能性

---

## RBAC: install manifests のままだと動かない例がある

`kro-core-install-manifests.yaml` の `kro:controller` は **aggregation** 方式で最小権限です。
そのままだと、生成 CRD を kro が `list/watch` できず dynamic controller がエラーになりました。

今回の検証では、簡略化のため以下を追加して **事実上 unrestricted** にしました:

```bash
kubectl --context kind-kro-examples apply -f - <<'EOF'
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kro:controller:unrestricted
  labels:
    rbac.kro.run/aggregate-to-controller: "true"
rules:
  - apiGroups: ["*"]
    resources: ["*"]
    verbs: ["*"]
EOF
```

---

## examples 検証結果 (kind)

### 01-basic-webapp

- 結果: OK (RGD Active / instance Ready)
- 修正: `refs/kro/examples/01-basic-webapp/rgd.yaml`
  - `spec.schema.status.url` を `schema.*` 参照 + 文字列テンプレートから、Service 参照 + CEL 1式へ変更

確認:

- `WebApp hello-web` が `Ready=True`
- Deployment/Service が作成される

### 02-webapp-ingress-and-config

- 結果: OK (RGD Active / instance Ready)
- 修正: `refs/kro/examples/02-webapp-ingress-and-config/rgd.yaml`
  - `spec.schema.status.url` を Service 参照 + CEL 1式へ変更

注意:

- kind には Ingress controller を入れていないため、Ingress 自体は作成できても address/hostname は埋まりません。

### 03-postgres-readywhen

- 結果: OK (RGD Active / instance Ready)
- 修正: `refs/kro/examples/03-postgres-readywhen/rgd.yaml`
  - `spec.schema.status.connectionString` を CEL 1式へ変更
  - `spec.schema.status.dbReady` は bool が instance に出ないため、`dbReadyReplicas`(integer) を出すように変更
  - `readyWhen` / env var で参照していた `readyReplicas` を optional (`?` + `.orValue`) に変更

既知:

- `status.dbReady`(boolean) が **instance に出てこない**。
  `dbReady2` 等の別名でも同様で、bool 系 status field が materialize されない挙動を確認。
  一方で integer の `${dbStatefulSet.?status.?readyReplicas.orValue(0)}` は出力されるため、
  暫定回避としては `dbReadyReplicas` のように数値で出すのが安全そうです。

### 04-rgd-chaining-fullstack

- 結果: OK (RGD chaining / instance Ready)
- 修正:
  - `refs/kro/examples/04-rgd-chaining-fullstack/01-database-rgd.yaml`
    - `connectionString` を CEL 1式へ
    - `ready` / `readyWhen` を optional 参照へ
  - `refs/kro/examples/04-rgd-chaining-fullstack/02-webapp-rgd.yaml`
    - Deployment の `readyWhen` を `deployment.spec.replicas` 参照へ
    - `spec.schema.status.endpoint` を Service ベースへ(文字列テンプレート回避)
  - `refs/kro/examples/04-rgd-chaining-fullstack/03-fullstack-rgd.yaml`
    - 子 `DemoDatabase` の ready 判定を `database.status.state == "ACTIVE"` へ変更

背景:

- 親 RGD の `readyWhen` で子 CR の `status.<field>` を直接参照すると、field 未生成タイミングで `no such key` が出ることがありました。
  `status.state` は kro が常に持つため、ここでは state を使うのが安全でした。

### 05-multi-tenant

- 結果: NG (kind 上で `DemoTenantEnvironment` が `IN_PROGRESS` から進まない)
- 症状:
  - Namespace/ResourceQuota/NetworkPolicy は作成される
  - しかし `DemoTenantEnvironment` の `ResourcesReady` が `Unknown(ResourcesInProgress)` のまま固定される
  - 同様の現象を「Namespace + NetworkPolicy」でも再現
  - 「Namespace + ResourceQuota + NetworkPolicy」でも再現

推測(強め):

- kro v0.7.1 の readiness 判定が `NetworkPolicy` で詰まる(= kstatus Unknown を許容しない)可能性があります。

### 06-08 bucket 系

- 結果: 未検証
- 理由:
  - ACK / Config Connector / Azure Service Operator などの **外部 CRD/コントローラ** と **クラウド認証** が必要
  - kind の素のクラスタでは静的解析で `apiVersion/kind が存在しない` 扱いになり `ResourceGraphAccepted=False` になり得る

修正は実施済み(未実行):

- `refs/kro/examples/06-multi-cloud-bucket/01-rgd-awsbucket.yaml`
- `refs/kro/examples/06-multi-cloud-bucket/03-rgd-azurebucket.yaml`
- `refs/kro/examples/06-multi-cloud-bucket/04-rgd-bucket.yaml`
- `refs/kro/examples/07-portable-bucket-direct/rgd.yaml`
- `refs/kro/examples/08-bucket-cluster-specific/bucket-rgd-aws.yaml`
- `refs/kro/examples/08-bucket-cluster-specific/bucket-rgd-gcp.yaml`

---

## kind + kro のセットアップ手順 (再現用)

### 1) kubectl / kind を用意

例(amd64):

```bash
INSTALL_DIR=~/.local/share/omarchy/bin

# kubectl
KUBECTL_VERSION=v1.35.0
curl -fsSL -o /tmp/kubectl https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl
chmod +x /tmp/kubectl
mv /tmp/kubectl ${INSTALL_DIR}/kubectl

# kind
KIND_VERSION=v0.31.0
curl -fsSL -o /tmp/kind https://kind.sigs.k8s.io/dl/${KIND_VERSION}/kind-linux-amd64
chmod +x /tmp/kind
mv /tmp/kind ${INSTALL_DIR}/kind
```

### 2) kind cluster 作成

```bash
kind create cluster --name kro-examples --wait 60s
kubectl --context kind-kro-examples get nodes
```

### 3) kro を raw manifests でインストール

```bash
KRO_VERSION=0.7.1
KRO_VARIANT=kro-core-install-manifests

kubectl --context kind-kro-examples create namespace kro-system
kubectl --context kind-kro-examples apply -f \
  https://github.com/kubernetes-sigs/kro/releases/download/v${KRO_VERSION}/${KRO_VARIANT}.yaml

kubectl --context kind-kro-examples rollout status -n kro-system deploy/kro
```

### 4) (検証用) RBAC を緩める

上記の `kro:controller:unrestricted` を適用。

### 5) cleanup

```bash
kind delete cluster --name kro-examples
```

---

## TODO

### 対応中

- `refs/kro/examples/05-multi-tenant/` が kind で `IN_PROGRESS` 固定になる原因を追加調査
  - `NetworkPolicy` を含むグラフで再現(単独でも再現)するため、kro 実装側の readiness 判定(Unknown/kstatus)の確認が必要
- `refs/kro/examples/03-postgres-readywhen/` の `status.dbReady` が出てこない原因を追加調査
  - bool status field の materialize 挙動の最小再現が必要 (同式で integer は出る)

### 対応予定

- bucket 系(06/07/08)の実動作確認
  - 最低でも対象 CRD だけを入れた「dry run 的」な静的解析パスを通す
  - 可能なら各 provider の controller + 認証まで含めた検証環境で reconcile まで確認
- `spec.schema.status` の CEL 環境(利用可能な変数一覧)を kro 実装から確定させ、ドキュメントへフィードバック(差分の整理)
- `spec.schema.status` の文字列テンプレート挙動(リテラル欠落)を最小再現して upstream issue 化
- examples に対して CI で `kubectl apply` + `kubectl wait` を回す仕組みを追加
