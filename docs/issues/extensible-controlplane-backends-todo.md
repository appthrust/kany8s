# Extensible ControlPlane Backends TODO

参照:

- `docs/issues/extensible-controlplane-backends.md`
- `docs/adr/0011-extensible-controlplane-backends.md`

目的:

- `Kany8sControlPlane` (facade) + `Kany8sCluster` だけを apply すれば、backend (kro / kubeadm / external) に依らず Cluster API contract を満たしつつクラスタ作成フローを成立させる

## やったこと（このセッション）

- [x] dynamic watch の GVK->GVR 解決を RESTMapper 優先にし、kind->plural 推測依存を緩和（`internal/dynamicwatch/watcher.go`）
- [x] `Kany8sControlPlane` controller で owner `Cluster` を解決し、kro instance に `cluster.x-k8s.io/cluster-name` label を付与（`internal/controller/kany8scontrolplane_controller.go`）
- [x] kubeconfig Secret の `<cluster>-kubeconfig` の `<cluster>` を owner `Cluster` 名に寄せる（`internal/controller/kany8scontrolplane_controller.go`）
- [x] `Kany8sControlPlane` controller の RBAC marker に `clusters get/list/watch` を追加（`internal/controller/kany8scontrolplane_controller.go`）
- [x] `Kany8sControlPlane.spec` に backend 選択フィールドの雛形を追加（kro/kubeadm/external）、kro selector を optional pointer 化（`api/v1alpha1/kany8scontrolplane_types.go`）
- [x] `Kany8sControlPlaneTemplate` 側も backend 選択フィールドを追加、kro selector を optional pointer 化（`api/v1alpha1/kany8scontrolplanetemplate_types.go`）
- [x] unit tests の追従（pointer 化/owner Cluster 参照の前提追加など）

注記:

- `go test ./...` は envtest バイナリ未配置だと `internal/devtools` が落ちることがある（`make test`/`make setup-envtest` 前提）

## やること

### 1) API/Validation（"exactly one backend" を成立させる）

- [x] webhook validation: `spec.resourceGraphDefinitionRef` / `spec.kubeadm` / `spec.externalBackend` が "ちょうど 1 つ" であることを強制
- [x] webhook validation: backend 種別の作成後変更を禁止（immutable）
- [x] CRD/RBAC 反映: `make manifests generate` を実行して生成物を更新（`config/crd/bases/*`, `config/rbac/*`）

### 2) kubeadm backend を facade の backend として成立させる

- [x] facade が `Kany8sKubeadmControlPlane` を 1:1 で作成/更新（spec.version 注入、machineTemplate 等の投影）
- [x] OwnerReferences: `Kany8sKubeadmControlPlane` が owner `Cluster` を解決できるように、backend object に `Cluster` の OwnerReference も付与（controller ownerRef は facade に残す）
- [x] Status adapter: kubeadm backend の ready/endpoint/initialized/failure を facade (`Kany8sControlPlane`) に反映
- [x] kubeconfig: kubeadm backend が `<cluster>-kubeconfig` を直接作る前提（Option A）と facade 側の Option B の整合を決めて実装

### 3) external backend を facade の backend として成立させる

- [x] facade が external backend object を 1:1 で作成/更新（arbitrary spec の pass-through + `spec.version`/cluster identity の注入）
- [x] watch: external backend の GVK を動的 watch に登録し、status 変化で facade reconcile が走ることを確認
- [x] RBAC: external backend resource を watch/get/list するための install-time RBAC extension 方針（どこにどう定義するか）を整理
- [x] kubeconfigSecretRef の cross-namespace 制約（opt-in/allowlist/RBAC）を実装方針として確定

### 4) サンプル/受け入れテスト

- [x] `config/samples/controlplane_v1alpha1_kany8scontrolplane.yaml` を backend 選択 shape に更新（kro/kubeadm/external の例）
- [x] acceptance tests: kro / kubeadm の両方で "Cluster + Kany8sControlPlane (+ Kany8sCluster) だけ apply" が成立することを確認

## 実装メモ（この更新で追加）

- `Kany8sControlPlane` / `Kany8sControlPlaneTemplate` validating webhook を追加し、exactly-one backend と backend type immutable を強制
- facade controller に `kubeadm` / `external` backend reconciliation を実装
- external backend は dynamic watch 登録 + arbitrary spec pass-through + `spec.version` / `spec.cluster*` 注入を実装
- kubeconfig source secret は default で同一 namespace のみ許可（cross-namespace は拒否）
- self-managed acceptance template を `Kany8sControlPlane + spec.kubeadm` 形に更新

## 未解決メモ（設計に影響する論点）

- kubeadm backend の endpoint source-of-truth（infra の `spec.controlPlaneEndpoint` に寄せる前提）と、`Kany8sCluster` を infrastructureRef にする場合の整合
- facade が作る kubeconfig Secret の命名規則（`<cluster>-kubeconfig` の `<cluster>` を何と定義するか）
