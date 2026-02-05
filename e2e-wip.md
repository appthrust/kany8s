# E2E WIP (acceptance: self-managed CAPD + kubeadm)

このファイルは、`docs/PRD.md` の UC0 (CAPD(docker) + kubeadm による self-managed Kubernetes を作る) を
「実際に kind 上で管理クラスタを立てて、kany8s(ControlPlaneProvider) で workload cluster が作れて kubeconfig で接続できる」まで持っていくための作業ログ。

目的は「後から同じことを再現できる」こと。進捗/問題/修正/知見を一箇所に集約する。

## 受け入れ条件 (PRD)

`docs/PRD.md` の Must (self-managed) より:

- `Cluster Available=True`
- `RemoteConnectionProbe=True`
- `clusterctl get kubeconfig` で kubeconfig が取れて接続できる
- `kubectl get nodes` が成功する (NoWorkers は許容)

## 環境

- 日付: 2026-02-02
- docker: 29.1.3
- kind: v0.31.0 (node image: `kindest/node:v1.35.0`)
- kubectl(client): v1.35.0
- go: 1.25.5
- clusterctl: v1.12.2

## 現在の状況 (いま)

デバッグ用に残している kind 管理クラスタ上で、主要条件まで到達している。

- kind: `kany8s-acceptance-self-managed-20260202172529`
- Cluster: `default/demo-self-managed-docker`
  - `RemoteConnectionProbe=True`
  - `Available=True`
  - `ControlPlaneInitialized=True`
- Kany8sKubeadmControlPlane: `default/demo-self-managed-docker`
  - `status.initialization.controlPlaneInitialized=true`
  - `Ready=True`
- workload:
  - kubeconfig で接続でき `kubectl get nodes` は成功
  - node は `NotReady` のまま (理由: `cni plugin not initialized`)

注意:

- 上記 kind クラスタでは、デバッグ目的で一度 `Kany8sKubeadmControlPlane.status.initialization.controlPlaneInitialized=true` を手動 patch して挙動検証している。
- その後「RemoteConnectionProbe=True をトリガーに初期化を立てる」実装/テストを追加済み。新しい管理クラスタでは手動 patch 無しで到達できる状態を目指している。

## 現在やっていること

- 受け入れスクリプト `hack/acceptance-test-capd-kubeadm.sh` を「進捗が見える」「詰まったら原因が残る」形に改善しつつ、
  新規 kind 管理クラスタで `make test-acceptance-capd-kubeadm` を最後まで通してログを確定させる。

## ここまでに実施したこと (実績)

### 1) smoke e2e

- 実行: `make test-e2e`
- 結果: 2 specs / 2 passed (controller image build+deploy + metrics 200 OK)
- 注意: `make deploy` / `make build-installer` は `config/manager/kustomization.yaml` を変更するため、必要に応じて `git restore config/manager/kustomization.yaml` を行う

### 2) self-managed acceptance (初回の試行)

- 実行: `make test-acceptance-capd-kubeadm-keep`
- kind 管理クラスタを残した:
  - KIND_CLUSTER_NAME: `kany8s-acceptance-self-managed-20260202155212`
  - kubeconfig: `/tmp/kany8s-acceptance-self-managed-20260202155212/kubeconfig`
  - log: `/tmp/kany8s-acceptance-self-managed-20260202155212/acceptance-self-managed.log`

この時点で判明した問題:

#### 2.1) clusterctl の local provider 参照 (file://) がそのままだと失敗

- 症状: `clusterctl init ...` が `invalid version: "dist"` / `path format {basepath}/{provider-name}/{version}/{components.yaml}`
- 原因: `url: file:///.../dist/install.yaml` は clusterctl の期待する provider repository レイアウトではない
- 対策:
  - `{basepath}/{provider-label}/{version}/metadata.yaml` と `{components}.yaml` を用意する
  - 例: `control-plane-kany8s/v0.0.0/metadata.yaml` + `control-plane-components.yaml`

#### 2.2) CAPD が docker.sock に接続できない

- 症状: `DockerCluster` の `LoadBalancerAvailable` が失敗 (`Cannot connect to the Docker daemon at unix:///var/run/docker.sock`)
- 原因: kind node container に `/var/run/docker.sock` が無く、CAPD が hostPath mount しても socket に到達できない
- 対策: kind 作成時に `extraMounts` で host の `/var/run/docker.sock` を node に mount
  - `hack/acceptance-test-capd-kubeadm.sh` で `--config <kind-config.yaml>` を使う
  - mount の実体確認として `docker exec <node> test -S /var/run/docker.sock` を追加

#### 2.3) CAPD webhook が ready になる前に apply して connection refused

- 症状: DockerCluster/DockerMachineTemplate の webhook 呼び出しで `connect: connection refused`
- 対策:
  - `clusterctl init --wait-providers` 後に、provider Deployment の rollout を明示的に待つ
  - `capd-webhook-service` の endpoints が生えるまで待つ

#### 2.4) `examples/self-managed-docker/cluster.yaml` の不整合

- `DockerCluster`/`DockerMachineTemplate` が v1beta1 で webhook conversion/validation 絡みのノイズが増える
  - 対策: `infrastructure.cluster.x-k8s.io/v1beta2` に更新
- `Kany8sKubeadmControlPlane.spec.machineTemplate.infrastructureRef` に `apiVersion` を書いていた
  - 対策: `ContractVersionedObjectReference` は `apiGroup/kind/name` が正 (version は contract label から解決される)
- `kubeadmConfigSpec.clusterConfiguration.networking` など、CRD が受け付けないフィールドがあった
  - 対策: 最小構成に削る (今後 CNI インストールを入れるなら、対応フィールドで postKubeadmCommands 等に寄せる)

### 3) 修正 (repo): Kany8sKubeadmControlPlane CRD の contract label

目的:

- CAPI が `Kany8sKubeadmControlPlane` の apiVersion を contract(v1beta2)として解決できるようにする

対応:

- `api/v1alpha1/kany8skubeadmcontrolplane_types.go` に `+kubebuilder:metadata:labels="cluster.x-k8s.io/v1beta2=v1alpha1"` を追加
- `make manifests` で CRD 再生成
- 回帰防止: `internal/devtools/kany8skubeadmcontrolplane_api_test.go` で marker/CRD label を検査

### 4) 修正 (repo): kany8s-manager-role の RBAC に CRD 読み取りが必要

問題:

- `kany8s-controller-manager` ログに `customresourcedefinitions.apiextensions.k8s.io is forbidden` が出て、reconcile が進まず
  `demo-self-managed-docker-kubeconfig` Secret / `Machine` / `KubeadmConfig` が作られず `RemoteConnectionProbe` が met しない

原因:

- `external.GetObjectFromContractVersionedRef` が参照先 CRD の contract label を読むために CRD の get/list/watch を必要とする

対応:

- `internal/controller/controlplane/kany8skubeadmcontrolplane_controller.go` に RBAC marker 追加:
  - `+kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch`
- `make manifests` で `config/rbac/role.yaml` を再生成
- 回帰防止:
  - `internal/devtools/rbac_role_test.go` に CRD 読み取り権限の検査を追加

### 5) 修正 (repo): ControlPlaneInitialized の判定を RemoteConnectionProbe に寄せる

背景:

- `Kany8sKubeadmControlPlane` が `status.initialization.controlPlaneInitialized=true` を立てないと
  CAPI 側の `Cluster Available=True` が met しない
- 従来実装は「control plane Machine が Ready になったら initialized」としていた
  - しかし NodeReady は CNI 未導入だと `NotReady` のままになりやすく、NoWorkers 許容な受け入れ条件と相性が悪い

対応:

- `Cluster.status.conditions[RemoteConnectionProbe]==True` をトリガーに `controlPlaneInitialized=true` を立てる
  - 実装: `internal/controller/controlplane/kany8skubeadmcontrolplane_controller.go`
- ユニットテストで固定:
  - `internal/controller/controlplane/kany8skubeadmcontrolplane_reconciler_test.go`

## 受け入れスクリプトの改善 (進捗可視化 / 再現性)

対象: `hack/acceptance-test-capd-kubeadm.sh`

- kind 作成:
  - host の `/var/run/docker.sock` を kind node に mount
  - node 内に socket があることを `docker exec` で確認
- clusterctl:
  - `make build-installer` の出力 `dist/install.yaml` を artifacts 配下の local repo レイアウトへ配置
  - `metadata.yaml` を生成して `clusterctl init` が解釈できるようにする
- CAPD webhook:
  - provider Deployment rollout を待つ
  - `capd-webhook-service` の endpoints を待つ
- 進捗出力:
  - `kubectl wait` 中に 15 秒おきで Cluster/ControlPlane/Machine の状態スナップショットを出す
  - `ARTIFACTS_DIR` と log path を冒頭で必ず出す (tail しやすくする)

## 知識 / 知見 (重要)

### clusterctl の local provider URL

- `url: file:///path/to/dist/install.yaml` はそのままでは期待どおり動かない
- `control-plane-kany8s/<version>/metadata.yaml` と `control-plane-components.yaml` のレイアウトを用意する

### CAPD は kind で docker.sock が必須

- CAPD controller は `/var/run/docker.sock` を hostPath で参照する
- kind node 内に socket が無いと `Cannot connect to the Docker daemon` で止まる

### ContractVersionedObjectReference

- `apiVersion` ではなく `apiGroup/kind/name` を指定する
- version は CRD の contract label (`cluster.x-k8s.io/v1beta2: <apiVersion>`) から解決される

### NodeReady と CNI

- kubeadm は CNI を入れないため、CNI を入れないと node が `NotReady` になりやすい
- ただし `RemoteConnectionProbe` と kubeconfig 接続は node `NotReady` でも成立し得る

### RBAC: CRD 読み取り

- contract versioned ref 解決に CRD read が必要になるため、controller の ClusterRole に `customresourcedefinitions` の get/list/watch が必要

### Workload の Docker コンテナ残骸 (CA mismatch)

- workload 側の CAPD コンテナ (`demo-self-managed-docker-control-plane-0`, `demo-self-managed-docker-lb` など) は host docker に作られる
  - kind 管理クラスタ削除とは独立なので、kind を消しても残る
- 残骸がある状態で同名クラスタを再作成すると、kubeconfig Secret の CA と実クラスタの CA がズレて `RemoteConnectionProbe` が `x509: certificate signed by unknown authority` で失敗しやすい
- 対策:
  - 事前に `docker rm -f demo-self-managed-docker-control-plane-0 demo-self-managed-docker-lb` (および同 prefix) を実行
  - 受け入れスクリプト側でも検知してデフォルトで失敗させる (再現性/ハマり防止)

### CAPD webhook の起動直後レース (connection refused)

- `clusterctl init --wait-providers` 後でも、`capd-webhook-service` に最初の数回 `connection refused` が出ることがある
- 対策:
  - rollout + endpoints を明示的に待つ
  - sample apply は `connection refused` のときのみ短いリトライを入れる (fail-fast と両立)

### kind のクラスタ名が長すぎると作成に失敗

- `KIND_CLUSTER_NAME` が長すぎると `sethostname: invalid argument` で kind 作成が落ちることがある
  - kind node container 名/hostname に `"${KIND_CLUSTER_NAME}-control-plane"` が使われ、hostname の 63 文字制限に引っかかる
- 対策:
  - `KIND_CLUSTER_NAME` は短くする (例: `kany8s-sm-<timestamp>`)
  - 受け入れスクリプト側で長すぎる場合は早期にエラーにする

### kubeadm の certificatesDir

- CAPD/kindest node では、kubeadm bootstrap 時に証明書を `/etc/kubernetes/pki` に直接書く前提だと揺れやすい
- 対策: kubeadm の `certificatesDir` を `/run/kubeadm/pki` に寄せ、証明書注入も同ディレクトリへ揃える

## これからやること

1. 新しい kind 管理クラスタで `make test-acceptance-capd-kubeadm` を最後まで通し、ログ/成果物で受け入れ完遂を確認する
2. `e2e-wip.md` に「最終実行の artifacts/log/cluster 名」「コマンド」「結果」を追記する
3. (必要なら) CNI を入れて node を Ready にするか、受け入れ条件として node Ready を要求しない方針を明文化する

## 実行メモ (背景実行 / 進捗確認)

- スクリプトは `ARTIFACTS_DIR` と log path を冒頭に出すようにしている
- 背景実行したい場合の例:

```bash
# クラスタを残してデバッグする
CLEANUP=false make test-acceptance-capd-kubeadm > /tmp/kany8s-acceptance-capd-kubeadm-run.log 2>&1 &

# 進捗を見る
tail -f /tmp/kany8s-acceptance-self-managed-run.log
```

## 最終実行 (成功)

- 日付: 2026-02-03
- 実行:
  - `ARTIFACTS_DIR=/tmp/kany8s-acceptance-capd-kubeadm-clean-20260203080511 KIND_CLUSTER_NAME=kany8s-capd-20260203080511 CLEANUP=false make test-acceptance-capd-kubeadm`
- kind:
  - `kany8s-sm-20260203080511`
- artifacts:
  - `/tmp/kany8s-acceptance-self-managed-clean-20260203080511`
- management kubeconfig:
  - `/tmp/kany8s-acceptance-self-managed-clean-20260203080511/kubeconfig`
- log:
  - `/tmp/kany8s-acceptance-self-managed-clean-20260203080511/acceptance-self-managed.log`
- workload kubeconfig (script output):
  - `/tmp/kany8s-acceptance-self-managed-clean-20260203080511/workload.kubeconfig`
- 結果 (PRD):
  - `Cluster Available=True`
  - `RemoteConnectionProbe=True`
  - `clusterctl get kubeconfig` OK
  - `kubectl get nodes` OK (NoWorkers 許容 / node は `NotReady` のまま: `cni plugin not initialized`)
- 重要:
  - `status.initialization.controlPlaneInitialized` の手動 patch は不要
  - workload の Docker コンテナ (`demo-self-managed-docker-*`) は host docker に残り得るため、再実行時は事前削除が必要

## 最終実行 (成功 / clean + cleanup)

- 日付: 2026-02-03
- 実行:
  - `ARTIFACTS_DIR=/tmp/kany8s-acceptance-capd-kubeadm-clean-20260203090609 KIND_CLUSTER_NAME=kany8s-capd-20260203090609 CLEANUP=true make test-acceptance-capd-kubeadm`
- kind:
  - `kany8s-sm-20260203090609` (スクリプト終了時に削除)
- artifacts:
  - `/tmp/kany8s-acceptance-self-managed-clean-20260203090609`
- management kubeconfig:
  - `/tmp/kany8s-acceptance-self-managed-clean-20260203090609/kubeconfig`
- log:
  - `/tmp/kany8s-acceptance-self-managed-clean-20260203090609/acceptance-self-managed.log`
- workload kubeconfig (script output):
  - `/tmp/kany8s-acceptance-self-managed-clean-20260203090609/workload.kubeconfig`
- 結果 (PRD):
  - `Cluster Available=True`
  - `RemoteConnectionProbe=True`
  - `clusterctl get kubeconfig` OK
  - `kubectl get nodes` OK (NoWorkers 許容 / node は `NotReady` のまま: `cni plugin not initialized`)
- cleanup 確認:
  - `demo-self-managed-docker-*` (workload) の Docker コンテナはスクリプト終了時に削除されている
