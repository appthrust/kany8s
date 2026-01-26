# kro v0.7.1 issues (kind 検証メモ)

このファイルは upstream(GitHub) に issue を出すための下書き集です。
各セクションが「1 issue」想定で、期待挙動 / 実際の挙動 / 最小再現 / 回避策 をまとめています。

## How to use

- GitHub issue を作るときは、このファイルから「該当 Issue セクション」を本文にそのまま貼り付け
- タイトルは各 Issue セクションの見出し(またはタイトル案)をコピー
- `kubectl --context kind-kro-examples` は必要に応じて読み替え

関連:

- `kro.md` (検証全体のまとめ)
- `refs/kro/examples/` (検証対象の examples)

## 検証環境

- kind: v0.31.0
- Kubernetes: v1.35.0 (kindest/node:v1.35.0)
- kro: v0.7.1 (`kro-core-install-manifests.yaml`)
- context: `kind-kro-examples`
- RBAC: 検証のため `kro:controller:unrestricted` (aggregation) を追加

---

## v0.7.1: graphs containing NetworkPolicy never become Ready (ResourcesReady stays Unknown)

### Environment

- kind: v0.31.0
- Kubernetes: v1.35.0 (kindest/node:v1.35.0)
- kro: v0.7.1 (`kro-core-install-manifests.yaml`)
- RBAC: `kro:controller:unrestricted` を aggregation で付与(検証のため実質フルアクセス)

### Expected

- `NetworkPolicy` は readiness を持たないリソースなので、kro がデフォルトで Ready 扱いする
  (または、少なくとも `readyWhen` で Ready 判定を override できる)
- `NetworkPolicy` を含む instance が `Ready=True` / `state=ACTIVE` まで到達する

### Actual

- kind 上で `NetworkPolicy` を含む graph を作ると、下位リソースは作成されるが instance が `IN_PROGRESS` で止まり続ける
- `status.conditions` の `ResourcesReady` が `Unknown(ResourcesInProgress)` から変化しない
- `resources[].readyWhen: [${true}]` を付けても解消しない

例(conditions の抜粋):

```json
[
  {
    "type": "ResourcesReady",
    "status": "Unknown",
    "reason": "ResourcesInProgress",
    "message": "reconciling cluster mutation after apply"
  },
  {
    "type": "Ready",
    "status": "Unknown",
    "reason": "ResourcesInProgress",
    "message": "reconciling cluster mutation after apply"
  }
]
```

### Reproduction

```bash
kubectl --context kind-kro-examples apply -f - <<'EOF'
apiVersion: kro.run/v1alpha1
kind: ResourceGraphDefinition
metadata:
  name: issue-networkpolicy-never-ready.kro.run
spec:
  schema:
    apiVersion: v1alpha1
    kind: IssueNetworkPolicyNeverReady
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
      readyWhen:
        - ${true}
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

kubectl --context kind-kro-examples apply -f - <<'EOF'
apiVersion: kro.run/v1alpha1
kind: IssueNetworkPolicyNeverReady
metadata:
  name: issue-np-1
spec:
  name: issue-np-1
EOF

kubectl --context kind-kro-examples get issuenetworkpolicyneverreadies issue-np-1 -o wide
kubectl --context kind-kro-examples get issuenetworkpolicyneverreadies issue-np-1 -o jsonpath='{.status.conditions}' && echo

kubectl --context kind-kro-examples get ns issue-np-1 -o wide
kubectl --context kind-kro-examples -n issue-np-1 get networkpolicy demo -o wide
```

### Observed output (example)

```text
$ kubectl get issuenetworkpolicyneverreadies issue-np-1 -o wide
NAME         STATE         READY     AGE
issue-np-1   IN_PROGRESS   Unknown   53s

$ kubectl -n issue-np-1 get networkpolicy demo -o wide
NAME   POD-SELECTOR   AGE
demo   <none>         41s
```

### Notes

- 対照実験として `Namespace + ResourceQuota` は数秒で `Ready=True` になるため、`NetworkPolicy` 固有の問題に見えます。
- `kstatus` の判定が `Unknown` を許容しない/無限待ちになる、などの実装が原因の可能性があります。

### Workaround

- v0.7.1 では `NetworkPolicy` を kro 管理対象から外す(別の仕組みで配る)以外、実質的な回避がありませんでした。

---

## v0.7.1: `schema` is not defined in CEL env for spec.schema.status

### Environment

- kind: v0.31.0
- Kubernetes: v1.35.0 (kindest/node:v1.35.0)
- kro: v0.7.1 (`kro-core-install-manifests.yaml`)
- RBAC: `kro:controller:unrestricted` を aggregation で付与(検証のため実質フルアクセス)

### Expected

- `resources.template` と同様に、`spec.schema.status` でも `schema.*` を参照できる
  (spec/status を組み立てる際に input を使える)

### Actual

- `spec.schema.status` 内で `schema.*` を参照すると RGD が reject される

Error (example):

```text
failed to build resourcegraphdefinition 'issue-schema-var-in-status.kro.run': failed to build OpenAPI schema for instance status: failed to type-check status expression "schema.spec.name" at path "echoed": ERROR: <input>:1:1: undeclared reference to 'schema' (in container '')
 | schema.spec.name
 | ^
```

### Reproduction

```bash
kubectl --context kind-kro-examples apply -f - <<'EOF'
apiVersion: kro.run/v1alpha1
kind: ResourceGraphDefinition
metadata:
  name: issue-schema-var-in-status.kro.run
spec:
  schema:
    apiVersion: v1alpha1
    kind: IssueSchemaVarInStatus
    spec:
      name: string | required=true
    status:
      echoed: ${schema.spec.name}
  resources:
    - id: cm
      template:
        apiVersion: v1
        kind: ConfigMap
        metadata:
          name: ${schema.spec.name}
        data:
          hello: world
EOF

kubectl --context kind-kro-examples get rgd issue-schema-var-in-status.kro.run -o yaml
kubectl --context kind-kro-examples describe rgd issue-schema-var-in-status.kro.run
```

### Workaround

- `spec.schema.status` では `schema.*` を使わず、resource id 変数(例: `${service.metadata.name}`) から射影する

---

## v0.7.1: string templates in spec.schema.status drop literal prefix (e.g. 'http://')

### Environment

- kind: v0.31.0
- Kubernetes: v1.35.0 (kindest/node:v1.35.0)
- kro: v0.7.1 (`kro-core-install-manifests.yaml`)
- RBAC: `kro:controller:unrestricted` を aggregation で付与(検証のため実質フルアクセス)

### Expected

- `"http://${svc.metadata.name}"` のような文字列テンプレートが、そのまま 1 つの string として status に入る

### Actual

- リテラル部分が欠落し、`${...}` の評価結果だけが残る

例:

- 期待: `http://isst1`
- 実際: `isst1`

### Reproduction

```bash
kubectl --context kind-kro-examples apply -f - <<'EOF'
apiVersion: kro.run/v1alpha1
kind: ResourceGraphDefinition
metadata:
  name: issue-status-string-template.kro.run
spec:
  schema:
    apiVersion: v1alpha1
    kind: IssueStatusStringTemplate
    spec:
      name: string | required=true
    status:
      endpoint: "http://${svc.metadata.name}"
      endpointSafe: ${"http://" + svc.metadata.name}
  resources:
    - id: svc
      template:
        apiVersion: v1
        kind: Service
        metadata:
          name: ${schema.spec.name}
        spec:
          ports:
            - name: http
              port: 80
              targetPort: 80
          selector:
            app: ${schema.spec.name}
EOF

kubectl --context kind-kro-examples apply -f - <<'EOF'
apiVersion: kro.run/v1alpha1
kind: IssueStatusStringTemplate
metadata:
  name: isst1
spec:
  name: isst1
EOF

kubectl --context kind-kro-examples get issuestatusstringtemplates isst1 -o jsonpath='{.status.endpoint}' && echo
kubectl --context kind-kro-examples get issuestatusstringtemplates isst1 -o jsonpath='{.status.endpointSafe}' && echo
```

### Observed output (example)

```text
$ kubectl get issuestatusstringtemplates isst1 -o jsonpath='{.status.endpoint}'
isst1

$ kubectl get issuestatusstringtemplates isst1 -o jsonpath='{.status.endpointSafe}'
http://isst1
```

### Workaround

- status の string は「文字列テンプレート」ではなく、CEL 1 式の連結で返す
  - OK: `${"http://" + svc.metadata.name}`
  - NG: `"http://${svc.metadata.name}"`

---

## v0.7.1: instance status fields must refer to a resource (constants rejected)

### Environment

- kind: v0.31.0
- Kubernetes: v1.35.0 (kindest/node:v1.35.0)
- kro: v0.7.1 (`kro-core-install-manifests.yaml`)
- RBAC: `kro:controller:unrestricted` を aggregation で付与(検証のため実質フルアクセス)

### Expected

- `ok: ${true}` のように、定数や `schema.spec` のような input だけで status を作れる
  (version/feature flag などを status に出したい)

### Actual

- status field が resource を参照しない場合、RGD が reject される

エラー例:

```
instance status field must refer to a resource: status.ok
```

### Reproduction

```bash
kubectl --context kind-kro-examples apply -f - <<'EOF'
apiVersion: kro.run/v1alpha1
kind: ResourceGraphDefinition
metadata:
  name: issue-status-constant.kro.run
spec:
  schema:
    apiVersion: v1alpha1
    kind: IssueStatusConstant
    spec:
      name: string | required=true
    status:
      ok: ${true}
  resources:
    - id: svc
      template:
        apiVersion: v1
        kind: Service
        metadata:
          name: ${schema.spec.name}
        spec:
          ports:
            - port: 80
              targetPort: 80
          selector:
            app: ${schema.spec.name}
EOF

kubectl --context kind-kro-examples describe rgd issue-status-constant.kro.run
```

### Workaround

- status に定数を出したい場合は、何らかの resource フィールドを経由して無理やり参照を作る必要がある
  (ただし不要な依存を作るので推奨しづらい)

---

## v0.7.1: status field disappears when referencing optional resource (includeWhen=false)

### Environment

- kind: v0.31.0
- Kubernetes: v1.35.0 (kindest/node:v1.35.0)
- kro: v0.7.1 (`kro-core-install-manifests.yaml`)
- RBAC: `kro:controller:unrestricted` を aggregation で付与(検証のため実質フルアクセス)

### Expected

- optional resource が無いときでも、optional chaining + `.orValue()` で default 値を返せる
  - 例: `${optCm.?metadata.?name.orValue("none")}` -> `"none"`

### Actual

- resource が `includeWhen=false` で skip された場合、status field 自体が出力されない
  (default 値にならない)

### Reproduction

```bash
kubectl --context kind-kro-examples apply -f - <<'EOF'
apiVersion: kro.run/v1alpha1
kind: ResourceGraphDefinition
metadata:
  name: issue-optional-resource-status.kro.run
spec:
  schema:
    apiVersion: v1alpha1
    kind: IssueOptionalResourceStatus
    spec:
      name: string | required=true
      enabled: boolean | default=false
    status:
      optName: ${optCm.?metadata.?name.orValue("none")}
      always: ${svc.metadata.name}
  resources:
    - id: svc
      template:
        apiVersion: v1
        kind: Service
        metadata:
          name: ${schema.spec.name}
        spec:
          ports:
            - port: 80
              targetPort: 80
          selector:
            app: ${schema.spec.name}

    - id: optCm
      includeWhen:
        - ${schema.spec.enabled}
      template:
        apiVersion: v1
        kind: ConfigMap
        metadata:
          name: ${schema.spec.name}-opt
        data:
          hello: world
EOF

kubectl --context kind-kro-examples apply -f - <<'EOF'
apiVersion: kro.run/v1alpha1
kind: IssueOptionalResourceStatus
metadata:
  name: iors1
spec:
  name: iors1
  enabled: false
EOF

kubectl --context kind-kro-examples get issueoptionalresourcestatuses iors1 -o jsonpath='{.status}' && echo

# 期待: optName="none" が入る
# 実際: status に optName field 自体が無い
```

### Observed output (example)

```text
$ kubectl get issueoptionalresourcestatuses iors1 -o jsonpath='{.status}'
{"always":"iors1","conditions":[...],"state":"ACTIVE"}
```

### Workaround

- "常に出したい status" を optional resource に依存させない
- optional な status は「出たり出なかったり」を許容する

---

## v0.7.1: boolean status field can be missing until RGD update (but `int(<number>) == 1` works)

### Environment

- kind: v0.31.0
- Kubernetes: v1.35.0 (kindest/node:v1.35.0)
- kro: v0.7.1 (`kro-core-install-manifests.yaml`)
- RBAC: `kro:controller:unrestricted` を aggregation で付与(検証のため実質フルアクセス)

### Expected

- `dbReady: ${dbStatefulSet.?status.?readyReplicas.orValue(0) == 1}` のような boolean status field が instance status に出力される

### Actual

- `dbReady` が instance status に出力されない(欠落する)
- 同じ式を `string(...)` で string 化したもの(`dbOkStr`)は `"true"` で出力される
- `int(<number>) == 1` のように **数値を明示的に cast** すると boolean が出力される (`int(<bool>)` は kro に reject される)
- ただし RGD を更新して再 reconcile させると、欠落していた `dbReady` が出力される

### Reproduction

```bash
kubectl --context kind-kro-examples apply -f - <<'EOF'
apiVersion: kro.run/v1alpha1
kind: ResourceGraphDefinition
metadata:
  name: issue-dbready-missing.kro.run
spec:
  schema:
    apiVersion: v1alpha1
    kind: IssueDbReadyMissing
    spec:
      name: string | required=true
    status:
      dbReady: ${dbStatefulSet.?status.?readyReplicas.orValue(0) == 1}
      dbOkInt: ${int(dbStatefulSet.?status.?readyReplicas.orValue(0)) == 1}
      dbOkStr: ${string(dbStatefulSet.?status.?readyReplicas.orValue(0) == 1)}
      dbReadyReplicas: ${dbStatefulSet.?status.?readyReplicas.orValue(0)}
  resources:
    - id: dbSecret
      template:
        apiVersion: v1
        kind: Secret
        metadata:
          name: ${schema.spec.name}-db
        type: Opaque
        stringData:
          password: changeme

    - id: dbService
      template:
        apiVersion: v1
        kind: Service
        metadata:
          name: ${schema.spec.name}-db
        spec:
          ports:
            - name: postgres
              port: 5432
              targetPort: postgres
          selector:
            app: ${schema.spec.name}-db

    - id: dbStatefulSet
      readyWhen:
        - ${dbStatefulSet.?status.?readyReplicas.orValue(0) == 1}
      template:
        apiVersion: apps/v1
        kind: StatefulSet
        metadata:
          name: ${schema.spec.name}-db
          labels:
            app: ${schema.spec.name}-db
        spec:
          serviceName: ${dbService.metadata.name}
          replicas: 1
          selector:
            matchLabels:
              app: ${schema.spec.name}-db
          template:
            metadata:
              labels:
                app: ${schema.spec.name}-db
            spec:
              containers:
                - name: postgres
                  image: postgres:15
                  ports:
                    - name: postgres
                      containerPort: 5432
                  env:
                    - name: POSTGRES_USER
                      value: postgres
                    - name: POSTGRES_PASSWORD
                      valueFrom:
                        secretKeyRef:
                          name: ${dbSecret.metadata.name}
                          key: password
                  volumeMounts:
                    - name: data
                      mountPath: /var/lib/postgresql/data
              volumes:
                - name: data
                  emptyDir: {}
EOF

kubectl --context kind-kro-examples apply -f - <<'EOF'
apiVersion: kro.run/v1alpha1
kind: IssueDbReadyMissing
metadata:
  name: issue-dbready-1
spec:
  name: issue-dbready-1
EOF

# wait for ACTIVE/Ready
kubectl --context kind-kro-examples get issuedbreadymissings issue-dbready-1 -o wide

# dbOkInt/dbOkStr/dbReadyReplicas are present, but dbReady is missing
kubectl --context kind-kro-examples get issuedbreadymissings issue-dbready-1 \
  -o jsonpath='{.status.dbReady} {.status.dbOkInt} {.status.dbOkStr} {.status.dbReadyReplicas}' && echo

kubectl --context kind-kro-examples get issuedbreadymissings issue-dbready-1 -o jsonpath='{.status}' && echo

# force a reconcile by updating the RGD (here: tweak readyWhen)
kubectl --context kind-kro-examples patch rgd issue-dbready-missing.kro.run --type json \
  -p '[{"op":"replace","path":"/spec/resources/2/readyWhen/0","value":"${dbStatefulSet.?status.?readyReplicas.orValue(0) > 0}"}]'

sleep 15
kubectl --context kind-kro-examples get issuedbreadymissings issue-dbready-1 \
  -o jsonpath='{.status.dbReady} {.status.dbOkInt} {.status.dbOkStr} {.status.dbReadyReplicas}' && echo
kubectl --context kind-kro-examples get issuedbreadymissings issue-dbready-1 -o jsonpath='{.status}' && echo
```

### Observed output (example)

```text
$ kubectl get issuedbreadymissings issue-dbready-1 -o jsonpath='{.status.dbReady} {.status.dbOkInt} {.status.dbOkStr} {.status.dbReadyReplicas}'
 true true 1

$ kubectl get issuedbreadymissings issue-dbready-1 -o jsonpath='{.status}'
{"dbOkInt":true,"dbOkStr":"true","dbReadyReplicas":1,"state":"ACTIVE",...}

# After patching the RGD
$ kubectl get issuedbreadymissings issue-dbready-1 -o jsonpath='{.status.dbReady} {.status.dbOkInt} {.status.dbOkStr} {.status.dbReadyReplicas}'
true true true 1
```

### Notes

- Postgres(StatefulSet) + `readyWhen` を含む例でも同様に、bool status field が欠落するケースを観測しました。
  この IssueDbReadyMissing はその挙動を最小化した repro です。
- `readyWhen` を変える(= RGD を更新する)と出るため、managed resource の status 更新に追従して status field を再計算できていない可能性があります。

### Workaround

- `dbReady` を使わず `dbReadyReplicas` を出して `> 0` 判定する
- boolean を直接出す場合は `int(<number>) == 1` のように「数値側に cast」を入れる(この例では `dbOkInt`)。`int(<bool>)` は kro に reject される
