# E2E Verification Report (kind + kro + Kany8s + Cluster API)

このレポートは `e2e-guide.md` の手順に沿って、kind 上のフレッシュなクラスタで kro -> Kany8s の反映、および (任意) Cluster API の endpoint mirroring までを手動検証した記録です。

## 実行メタデータ

- 実行日時: 2026-01-26T17:26:45+09:00
- 対象コミット: `7e258cae2049213ff2bea35a2028952f051dba37` (`chore: add devbox environment`)

## 前提 / ツール

- docker: 29.1.3
- kind: v0.31.0
- kubectl: v1.35.0
- go: 1.25.7
- make: 4.4.1
- (Step 7 用) clusterctl: v1.12.2

## 変数 (今回の実行値)

ガイドの `KIND_CLUSTER_NAME=kany8s` は既存クラスタと衝突するため、今回の検証では一意な名前に変更しました。

```bash
export KIND_CLUSTER_NAME=kany8s-e2e-20260126
export KUBECTL_CONTEXT=kind-${KIND_CLUSTER_NAME}
export KRO_VERSION=0.7.1

export NAMESPACE=default
export CLUSTER_NAME=demo-cluster

# Controller image tag used for kind load + deploy
export IMG=example.com/kany8s:e2e-7e258ca
```

## スコープ / 期待値

- kro: RGD (`ResourceGraphDefinition`) が受理され (`ResourceGraphAccepted=True`)、instance CRD が生成される。
- Kany8s: kro instance の `status.ready` / `status.endpoint` を消費し、`Kany8sControlPlane` の endpoint / initialization / conditions に反映する。
- (任意) Cluster API: `Cluster.spec.controlPlaneEndpoint` が `Kany8sControlPlane` 由来の endpoint に追随する。

注意:

- デモ RGD (`examples/kro/ready-endpoint/`) は nginx の Deployment/Service を作るだけで実 API server ではないため、CAPI の `RemoteConnectionProbe` が失敗し `Cluster Available=False` は想定どおりです。

## Step 1) kind クラスタ作成

実行コマンド:

```bash
kind create cluster --name "${KIND_CLUSTER_NAME}" --wait 60s
kubectl --context "${KUBECTL_CONTEXT}" get nodes
```

確認結果:

```text
NAME                                STATUS   ROLES           AGE   VERSION
kany8s-e2e-20260126-control-plane   Ready    control-plane   24s   v1.35.0
```

## Step 2) kro v0.7.1 インストール

実行コマンド:

```bash
kubectl --context "${KUBECTL_CONTEXT}" create namespace kro-system
kubectl --context "${KUBECTL_CONTEXT}" apply -f \
  "https://github.com/kubernetes-sigs/kro/releases/download/v${KRO_VERSION}/kro-core-install-manifests.yaml"
kubectl --context "${KUBECTL_CONTEXT}" rollout status -n kro-system deploy/kro --timeout=180s
```

確認コマンド / 結果:

```bash
kubectl --context "${KUBECTL_CONTEXT}" -n kro-system get deploy kro -o wide
```

```text
NAME   READY   UP-TO-DATE   AVAILABLE   AGE   CONTAINERS   IMAGES                           SELECTOR
kro    1/1     1            1           17s   kro          registry.k8s.io/kro/kro:v0.7.1   app.kubernetes.io/instance=kro,app.kubernetes.io/name=kro
```

## Step 3) kro controller RBAC 緩和 (v0.7.1 workaround)

適用した YAML (inline):

```yaml
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
```

確認コマンド / 結果:

```bash
kubectl --context "${KUBECTL_CONTEXT}" get clusterrole kro:controller:unrestricted -o name
```

```text
clusterrole.rbac.authorization.k8s.io/kro:controller:unrestricted
```

## Step 4) demo RGD 適用と受理確認

実行コマンド:

```bash
kubectl --context "${KUBECTL_CONTEXT}" apply -f examples/kro/ready-endpoint/rgd.yaml
kubectl --context "${KUBECTL_CONTEXT}" wait \
  --for=condition=ResourceGraphAccepted \
  --timeout=120s \
  rgd/demo-control-plane.kro.run
```

受理 condition の出力 (jsonpath):

```text
True
Valid
resource graph and schema are valid
```

生成された instance CRD の確認:

```bash
kubectl --context "${KUBECTL_CONTEXT}" get crd democontrolplanes.kro.run -o name
```

```text
customresourcedefinition.apiextensions.k8s.io/democontrolplanes.kro.run
```

## Step 5) Kany8s の install + deploy (in-cluster)

実行コマンド:

```bash
kubectl config use-context "${KUBECTL_CONTEXT}"

make install

make docker-build IMG="${IMG}"
kind load docker-image "${IMG}" --name "${KIND_CLUSTER_NAME}"

make deploy IMG="${IMG}"
kubectl -n kany8s-system rollout status deployment/kany8s-controller-manager --timeout=180s
```

確認結果 (rollout):

```text
deployment "kany8s-controller-manager" successfully rolled out
```

補足 (repo cleanliness):

- `make deploy` は `config/manager/kustomization.yaml` に image を書き込みます。
- 今回はガイドどおり、検証後に `git restore config/manager/kustomization.yaml` を実行して差分を戻しました。

## Step 6) Kany8sControlPlane 適用と kro -> Kany8s 反映確認

適用した YAML (inline):

```yaml
apiVersion: controlplane.cluster.x-k8s.io/v1alpha1
kind: Kany8sControlPlane
metadata:
  name: demo-cluster
  namespace: default
spec:
  version: "1.34"
  resourceGraphDefinitionRef:
    name: demo-control-plane.kro.run
  kroSpec:
    name: demo-cluster
```

確認結果:

- `kany8scontrolplane/demo-cluster`:

```text
Ready=True
controlPlaneInitialized=true
endpoint.host=demo-cluster-svc.default.svc.cluster.local
endpoint.port=6443
failureReason/failureMessage=(empty)
```

- kro instance `democontrolplanes.kro.run/demo-cluster`:

```text
status.ready=true
status.endpoint=demo-cluster-svc.default.svc.cluster.local:6443
status.state=ACTIVE
```

## Step 7) (任意) Cluster API 導入と endpoint mirroring 確認

### Step 7.1 v1beta2 contract の確認/導入

事前状態:

- `clusters.cluster.x-k8s.io` CRD は未導入

実行コマンド:

```bash
curl -fsSL -o bin/clusterctl-v1.12.2 \
  https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.12.2/clusterctl-linux-amd64
chmod +x bin/clusterctl-v1.12.2

bin/clusterctl-v1.12.2 init --infrastructure docker --wait-providers --kubeconfig-context "${KUBECTL_CONTEXT}"
```

`Cluster` CRD の served versions 確認結果:

```text
v1alpha3:false,false
v1alpha4:false,false
v1beta1:true,false
v1beta2:true,true
```

### Step 7.2 InfrastructureRef (Kany8sCluster) 作成

適用した YAML (inline):

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: Kany8sCluster
metadata:
  name: demo-cluster
  namespace: default
spec: {}
```

### Step 7.3 Cluster apply と endpoint mirroring

実行コマンド:

```bash
kubectl --context "${KUBECTL_CONTEXT}" apply -f examples/capi/cluster.yaml

kubectl --context "${KUBECTL_CONTEXT}" -n "${NAMESPACE}" wait \
  --for=jsonpath='{.spec.controlPlaneEndpoint.host}'=demo-cluster-svc.default.svc.cluster.local \
  --timeout=240s \
  "cluster/${CLUSTER_NAME}"
```

確認結果:

- `Cluster.spec.controlPlaneEndpoint.host`: `demo-cluster-svc.default.svc.cluster.local`
- `Cluster.spec.controlPlaneEndpoint.port`: `6443`

参考 (Cluster conditions の抜粋):

```text
Available:False NotAvailable
RemoteConnectionProbe:False ProbeFailed
InfrastructureReady:True Ready
ControlPlaneInitialized:True Initialized
ControlPlaneAvailable:True Available
```

## Step 8) Cleanup

実行コマンド:

```bash
kind delete cluster --name "${KIND_CLUSTER_NAME}"
```

実行結果:

```text
Deleting cluster "kany8s-e2e-20260126" ...
Deleted nodes: ["kany8s-e2e-20260126-control-plane"]
```

補足:

- kind の context は削除され、`kubectl config current-context` は未設定状態になりました。

## 付録: 今回参照/適用した YAML

### `examples/kro/ready-endpoint/rgd.yaml`

```yaml
apiVersion: kro.run/v1alpha1
kind: ResourceGraphDefinition
metadata:
  name: demo-control-plane.kro.run
spec:
  schema:
    apiVersion: v1alpha1
    kind: DemoControlPlane
    spec:
      name: string | required=true description="Instance name"
      version: string | required=true description="Kubernetes version"
      image: string | default="nginx:1.27" description="Demo image"
      replicas: integer | default=1 minimum=1 maximum=10
      port: integer | default=6443 minimum=1 maximum=65535
    status:
      endpoint: ${service.metadata.name + "." + service.metadata.namespace + ".svc.cluster.local:" + string(service.spec.ports[0].port)}
      ready: ${int(deployment.?status.?availableReplicas.orValue(0)) > 0}

  resources:
    - id: deployment
      template:
        apiVersion: apps/v1
        kind: Deployment
        metadata:
          name: ${schema.spec.name}
          labels:
            app.kubernetes.io/name: ${schema.spec.name}
        spec:
          replicas: ${schema.spec.replicas}
          selector:
            matchLabels:
              app.kubernetes.io/name: ${schema.spec.name}
          template:
            metadata:
              labels:
                app.kubernetes.io/name: ${schema.spec.name}
            spec:
              containers:
                - name: control-plane
                  image: ${schema.spec.image}
                  ports:
                    - name: https
                      containerPort: ${schema.spec.port}

    - id: service
      template:
        apiVersion: v1
        kind: Service
        metadata:
          name: ${schema.spec.name}-svc
          labels:
            app.kubernetes.io/name: ${schema.spec.name}
        spec:
          selector: ${deployment.spec.selector.matchLabels}
          ports:
            - name: https
              protocol: TCP
              port: ${schema.spec.port}
              targetPort: ${schema.spec.port}
```

### `examples/capi/cluster.yaml`

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: demo-cluster
  namespace: default
spec:
  # Kany8s is currently a ControlPlane provider only. Replace this ref with your
  # Infrastructure provider (e.g. DockerCluster, AWSCluster, AzureCluster, ...).
  infrastructureRef:
    apiGroup: infrastructure.cluster.x-k8s.io
    kind: Kany8sCluster
    name: demo-cluster
  controlPlaneRef:
    apiGroup: controlplane.cluster.x-k8s.io
    kind: Kany8sControlPlane
    name: demo-cluster
---
apiVersion: controlplane.cluster.x-k8s.io/v1alpha1
kind: Kany8sControlPlane
metadata:
  name: demo-cluster
  namespace: default
spec:
  version: "1.34"
  resourceGraphDefinitionRef:
    name: demo-control-plane.kro.run
  kroSpec:
    # Demo RGD requires `spec.name`.
    name: demo-cluster
```

## 付録: "結果 YAML" を残す場合の dump コマンド (参考)

今回の run では `kubectl get -o yaml` のフルダンプは採取していません。次回、cleanup 前に以下を実行すると、実体 YAML をレポートに添付できます。

```bash
kubectl --context "${KUBECTL_CONTEXT}" -n kro-system get deploy kro -o yaml
kubectl --context "${KUBECTL_CONTEXT}" get rgd demo-control-plane.kro.run -o yaml
kubectl --context "${KUBECTL_CONTEXT}" get crd democontrolplanes.kro.run -o yaml

kubectl --context "${KUBECTL_CONTEXT}" -n kany8s-system get deploy kany8s-controller-manager -o yaml
kubectl --context "${KUBECTL_CONTEXT}" -n "${NAMESPACE}" get kany8scontrolplane "${CLUSTER_NAME}" -o yaml
kubectl --context "${KUBECTL_CONTEXT}" -n "${NAMESPACE}" get democontrolplanes.kro.run "${CLUSTER_NAME}" -o yaml

kubectl --context "${KUBECTL_CONTEXT}" -n "${NAMESPACE}" get kany8scluster "${CLUSTER_NAME}" -o yaml
kubectl --context "${KUBECTL_CONTEXT}" -n "${NAMESPACE}" get cluster "${CLUSTER_NAME}" -o yaml
```
