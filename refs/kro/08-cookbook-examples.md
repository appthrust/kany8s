# 08. Cookbook / Examples

この章は、公式 examples と概念ページをベースに「そのまま真似できる」パターンを整理します。

## 0) 事前準備

- kro をインストール済み (`refs/kro/02-installation.md`)

## 1) 最小(Noop) RGD

リソースを一切作らない RGD。構造の最小例です。

```yaml
apiVersion: kro.run/v1alpha1
kind: ResourceGraphDefinition
metadata:
  name: noop
spec:
  schema:
    apiVersion: v1alpha1
    kind: NoOp
    spec:
      name: string | required=true
  resources: []
```

## 2) Deployment + Service

```yaml
apiVersion: kro.run/v1alpha1
kind: ResourceGraphDefinition
metadata:
  name: deploymentservice
spec:
  schema:
    apiVersion: v1alpha1
    kind: DeploymentService
    spec:
      name: string
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
              app: deployment
          template:
            metadata:
              labels:
                app: deployment
            spec:
              containers:
                - name: ${schema.spec.name}-deployment
                  image: nginx
                  ports:
                    - containerPort: 80
    - id: service
      template:
        apiVersion: v1
        kind: Service
        metadata:
          name: ${schema.spec.name}
        spec:
          selector:
            app: deployment
          ports:
            - protocol: TCP
              port: 80
              targetPort: 80
```

インスタンス例:

```yaml
apiVersion: kro.run/v1alpha1
kind: DeploymentService
metadata:
  name: demo
spec:
  name: demo
```

## 3) Optional Ingress (includeWhen)

Ingress を `includeWhen` でオン/オフする例。

```yaml
schema:
  spec:
    name: string
    ingress:
      enabled: boolean | default=false
resources:
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

## 4) 外部 ConfigMap を参照する (externalRef + `?`)

### 参照される ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: demo
data:
  ECHO_VALUE: "Hello, World!"
```

### RGD

```yaml
resources:
  - id: input
    externalRef:
      apiVersion: v1
      kind: ConfigMap
      metadata:
        name: demo
        namespace: default
  - id: deployment
    template:
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: ${schema.spec.name}
      spec:
        template:
          spec:
            containers:
              - name: busybox
                image: busybox
                command: ["sh", "-c", "echo $MY_VALUE && sleep 3600"]
                env:
                  - name: MY_VALUE
                    value: ${input.data.?ECHO_VALUE}
```

ポイント

- ConfigMap `data` のキーはスキーマで確定しないため、`input.data.ECHO_VALUE` だと静的検証で失敗する可能性があります。
- `?` を使うと `null` になり得るので、必要なら `.orValue("...")` でデフォルトを付けます。

## 5) Secret の変換(base64 decode)

Secret の `.data` は base64 なので、CEL のエンコーダ関数で復元して加工する例です。

参照される Secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: test
stringData:
  uri: api.test.com
```

RGD(抜粋):

```yaml
resources:
  - id: test
    externalRef:
      apiVersion: v1
      kind: Secret
      metadata:
        name: test
        namespace: ""
  - id: secret
    template:
      apiVersion: v1
      kind: Secret
      metadata:
        name: ${schema.spec.name}
      stringData:
        token: "${ string(base64.decode(string(test.data.uri))) }/oauth/token"
```

## 6) Multi-tenant: Namespace + Quota + NetworkPolicy

テナントごとに namespace を作り、Quota と NetworkPolicy を付ける例(公式例の簡略版)。

```yaml
apiVersion: kro.run/v1alpha1
kind: ResourceGraphDefinition
metadata:
  name: tenantenvironment.kro.run
spec:
  schema:
    apiVersion: v1alpha1
    kind: TenantEnvironment
    spec:
      tenantId: string
  resources:
    - id: tenantNamespace
      template:
        apiVersion: v1
        kind: Namespace
        metadata:
          name: ${schema.spec.tenantId}

    - id: tenantQuota
      template:
        apiVersion: v1
        kind: ResourceQuota
        metadata:
          name: ${schema.spec.tenantId}-quota
          namespace: ${schema.spec.tenantId}
        spec:
          hard:
            requests.cpu: "1"
            requests.memory: "1Gi"
            limits.cpu: "2"
            limits.memory: "2Gi"

    - id: tenantNetworkPolicy
      template:
        apiVersion: networking.k8s.io/v1
        kind: NetworkPolicy
        metadata:
          name: ${schema.spec.tenantId}-isolation
          namespace: ${schema.spec.tenantId}
        spec:
          podSelector:
            matchLabels: {}
          ingress:
            - from:
                - podSelector: {}
```

注意

- cluster-scoped resource(Namespace) を管理するため、ownerReferences を付ける設計は相性が悪いです。
- RBAC を aggregation で運用する場合は `namespaces`, `resourcequotas`, `networkpolicies` の権限を kro に付与してください。

## 7) さらに大きな例

公式 examples には以下が含まれます。

- CoreDNS の再構成(ClusterRole/Binding/ConfigMap/Deployment/Service など)
- AWS ACK CRDs を使った EKS/VPC/Valkey/RDS など

学習の順としては「まず自分のクラスタに既にある CRD を 1 つ取り込み、status を返す」から始めるのが効果的です。
