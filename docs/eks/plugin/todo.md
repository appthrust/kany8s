# TODO: EKS plugins (kubeconfig rotator)

- 作成日: 2026-02-07
- ステータス: Open

この TODO は `docs/eks/plugin/eks-kubeconfig-rotator.md` の設計を、実装に落とすためのチェックリスト。

## 0) 設計確定 / 前提整理

- [x] 設計ドラフト作成: `docs/eks/plugin/eks-kubeconfig-rotator.md`
- [x] ACK(EKS) が token を提供しないことを確認（`status.endpoint` / `status.certificateAuthority.data` のみ）
- [x] token 生成方針を確定する
  - [x] Go 実装（`aws-iam-authenticator` の `pkg/token` を利用して `k8s-aws-v1.` token を生成）
  - [ ] (Optional) コンテナ内 `aws eks get-token` 実行（awscli 同梱）
  - [x] token TTL/refresh/expiration
    - [x] TTL: `aws-iam-authenticator` の実装に追随（実質 `15m - 1m`）
    - [x] refresh: `expiration - 5m` を目安、ただし上限間隔 `10m`
    - [x] expiration: Secret annotation `eks.kany8s.io/token-expiration-rfc3339` に保存
- [x] plugin が管理する Secret の責務を確定する
  - [x] CAPI probe 用: `<capiClusterName>-kubeconfig`（token 埋め込み）
  - [x] 人間用: `<capiClusterName>-kubeconfig-exec`（exec kubeconfig）
- [x] Kany8s facade との競合を避ける方針を確定する
  - [x] BYO RGD で `status.kubeconfigSecretRef` を出さない（Kany8s に `<capiClusterName>-kubeconfig` を作らせない）
  - [x] `<capiClusterName>-kubeconfig` の Owner は plugin（既存 Secret が別 owner の場合は上書きしない）

## 1) 入力 contract（opt-in/名前/region 解決）

- [x] opt-in のキーを確定する
  - [x] `Cluster.metadata.annotations["eks.kany8s.io/kubeconfig-rotator"] = "enabled"`
  - [ ] (Optional) `ClusterClass` 変数から annotation を注入する（Topology 利用者の UX を揃える）
- [x] EKS cluster 名の解決ルールを確定する
  - [x] `capiClusterName = Cluster.metadata.name`
  - [x] `eksClusterName = Cluster.metadata.annotations["eks.kany8s.io/cluster-name"] ?? (if controlPlaneRef.apiGroup=="controlplane.cluster.x-k8s.io" && controlPlaneRef.kind=="Kany8sControlPlane" then controlPlaneRef.name) ?? capiClusterName`
- [x] region 解決ルールを実装に落とす
  - [x] `Cluster.metadata.annotations["eks.kany8s.io/region"]`（明示）
  - [x] ACK EKS Cluster `status.ackResourceMetadata.region`
  - [x] ACK EKS Cluster `metadata.annotations["services.k8s.aws/region"]`（フォールバック）
- [x] ACK EKS Cluster の参照方法を確定する
  - [x] `ackClusterName = Cluster.metadata.annotations["eks.kany8s.io/ack-cluster-name"] ?? eksClusterName`
  - [x] ACK EKS Cluster は同一 namespace にある前提（cross-namespace は非対応）

- [ ] Topology/ClusterClass 環境での “名前のズレ” を吸収する
  - [x] 現状の BYO サンプルでは、ACK EKS Cluster の `metadata.name` / `spec.name` が `Cluster.metadata.name` ではなく `Cluster.spec.controlPlaneRef.name`（Kany8sControlPlane 名）になることがある
  - [x] annotation 未指定時のデフォルト解決を見直す
    - [x] `eksClusterName = cluster.annotations["eks.kany8s.io/cluster-name"] ?? (if controlPlaneRef.apiGroup=="controlplane.cluster.x-k8s.io" && controlPlaneRef.kind==Kany8sControlPlane then controlPlaneRef.name) ?? cluster.name`
    - [x] `ackClusterName = cluster.annotations["eks.kany8s.io/ack-cluster-name"] ?? eksClusterName`
  - [x] 仕様（どの名前を “EKS 上の cluster 名” と見なすか）を `docs/eks/plugin/eks-kubeconfig-rotator.md` に追記する
  - [ ] (関連) `docs/eks/issues/eks-byo-network-sample-followups.md` の ClusterClass naming 固定と整合を取る

## 2) Kubernetes controller 実装（骨格）

- [x] 実装場所（repo 内/外）を決める
  - [x] repo 内に新規 binary: `cmd/eks-kubeconfig-rotator/main.go`
  - [ ] あるいは別 repo（この repo では docs のみ保持）
- [x] controller-runtime manager を立てる（metrics/healthz/leader election）
- [x] watch 対象を決める
  - [x] CAPI `cluster.x-k8s.io/v1beta2 Cluster`
  - [x] (推奨) ACK EKS `clusters.eks.services.k8s.aws/v1alpha1 Cluster`（unstructured でも可）
  - [ ] (任意) `Secret/<cluster>-kubeconfig`（自分が管理しているもののみ）
- [x] Reconciler の基本フローを実装する
  - [x] opt-in されていなければ何もしない
  - [x] cluster が削除中なら何もしない（OwnerRef で GC される設計なら）
  - [x] ACK EKS Cluster を取得し、`status.endpoint` / `status.certificateAuthority.data` を読む
  - [x] endpoint/CA が未確定なら requeue
  - [x] token を生成する
  - [x] kubeconfig を組み立てる（`clientcmd/api` を使って YAML を生成）
  - [x] `<cluster>-kubeconfig` Secret を CreateOrUpdate
  - [x] 次の refresh 時刻を計算して `RequeueAfter` を返す
- [x] Secret のメタデータ統一
  - [x] `type: cluster.x-k8s.io/secret`
  - [x] `labels["cluster.x-k8s.io/cluster-name"] = <cluster>`
  - [x] `data.value = <kubeconfig bytes>`
  - [x] OwnerReference（候補: `Cluster`）
  - [x] plugin 管理であることを示す annotation（例: `eks.kany8s.io/managed-by=eks-kubeconfig-rotator`）
- [x] CAPI ClusterCache の挙動を前提にした requeue を調整する
  - [x] token refresh は 10s 毎の probe より十分手前に行う（例: expire 5m 前）
  - [x] 失敗時の backoff（AWS/ACK 未同期など）
- [ ] Event/ログの最小セットを入れる
  - [x] token 生成失敗（credentials 不足など）
  - [x] ACK cluster 未作成/未同期
  - [ ] Secret 更新（頻度が高いので rate-limit する）

- [x] 名前解決ロジックを Topology 対応する
  - [x] `resolveClusterNames` を更新し、annotation 未指定時は `Cluster.spec.controlPlaneRef` を参照する
  - [x] `controlPlaneRef` の `apiGroup/kind` を確認し、想定外の CP の場合は従来通り `cluster.name` を使う（誤マッチ防止）
  - [x] ACK Cluster watch の mapping (`mapACKClusterToCAPIClusters`) も同じ解決ロジックを使う

## 3) token 生成の実装（詳細）

- [x] token 生成の実装を確定し、API を切り出す（テストしやすい形）
- [x] token と expiration を取得する
  - [x] Go 実装なら expiration を自前で計算し、Secret annotation に保存する
  - [ ] awscli 実行なら `aws eks get-token` の `expirationTimestamp` を採用する
- [ ] 生成した token で Kubernetes API が通る前提条件を整理する
  - [ ] `bootstrapClusterCreatorAdminPermissions: true` の場合、EKS 作成 principal と同一 principal で token を生成する必要がある
  - [ ] kind 手順では ACK と同じ `aws-creds` を plugin にも渡す方針を docs へ落とす

## 4) デプロイ（manifest/配布）

- [x] kustomize/manifest を追加する（置き場所を決める）
  - [x] `config/eks-plugin/` もしくは `config/overlays/eks-plugin/`
- [x] RBAC
  - [x] read: `cluster.x-k8s.io/clusters`
  - [x] read: `eks.services.k8s.aws/clusters`（ACK を読む場合）
  - [x] write: core `secrets`
  - [x] write: core `events`
- [x] leader election を使う場合の RBAC を追加する
  - [x] `coordination.k8s.io/leases`（namespaced Role + RoleBinding）
  - [ ] (必要なら) `configmaps`（古い leader election 実装向け）
- [x] secure metrics を有効にする場合の RBAC/設定を追加する
  - [ ] `authentication.k8s.io/tokenreviews` 作成
  - [ ] `authorization.k8s.io/subjectaccessreviews` 作成
  - [x] もしくは `--metrics-secure=false` / `--metrics-bind-address=0` で依存を外す
- [ ] AWS credentials の渡し方を決めて manifest 化する
  - [x] kind: `aws-creds` Secret を volume mount（ACK と同じ方式）
  - [ ] real cluster: IRSA を推奨（ServiceAccount + role）
  - [ ] env var/secret 名/ファイルパスを固定し、docs に書く
- [x] kind の Secret namespace 差を吸収する
  - [x] ACK 手順は `ack-system/aws-creds` を作るが、plugin は `kany8s-system/aws-creds` を参照している
  - [x] どちらの namespace に寄せるか決めて、manifest と docs を揃える
- [x] runtime 設定
  - [x] refresh 間隔/expire threshold を flag/env で設定可能にする
  - [x] namespace watch 範囲（全 namespace or 1 namespace）
- [x] image 差し替え導線を整備する
  - [x] `config/eks-plugin/kustomization.yaml` に `images:` を追加して差し替え可能にする
  - [x] もしくは `make deploy-eks-plugin` で image を差し替えられる仕組みを追加

## 5) テスト

- [x] unit
  - [x] kubeconfig builder（endpoint/CA/token から期待 YAML が出る）
  - [x] expiration/requeue 計算
  - [ ] (Go 実装の場合) token generator（固定時刻で deterministic）

- [x] unit: Topology 名ズレケース
  - [x] `Cluster.metadata.name != Cluster.spec.controlPlaneRef.name` のとき、annotation 未指定でも ACK EKS Cluster を `controlPlaneRef.name` で引ける
  - [x] そのとき token generator が `eksClusterName=controlPlaneRef.name` で呼ばれる
- [ ] envtest
  - [ ] Cluster + ACK(EKS) unstructured を与えて Secret が作られる
  - [ ] endpoint 未確定 -> requeue の挙動
- [ ] manual/e2e（kind + ACK + kro + CAPI core）
  - [ ] BYO cluster 作成 -> `<cluster>-kubeconfig` 生成
  - [ ] CAPI `RemoteConnectionProbe=True` を確認
  - [ ] `Cluster Available=True`（TopologyReconciled/InfrastructureReady/ControlPlaneAvailable/WorkersAvailable 含む）を確認
  - [ ] token 期限後も `RemoteConnectionProbe` が落ちない（refresh が効く）ことを確認

## 6) docs 更新

- [ ] `docs/eks/byo-network/README.md` に plugin 導入手順を追加（Available=True を狙う場合）
- [ ] `docs/eks/issues/eks-byo-network-sample-followups.md` の TODO を plugin 前提で更新
- [ ] `docs/eks/README.md` に plugin の位置づけ（probe/Available のための追加物）を追記

## Review follow-ups

- [x] plugin の RBAC を二重管理しない
  - [x] plugin controller の `// +kubebuilder:rbac` marker は、本体 `config/rbac/role.yaml` を汚す可能性があるため削除する
  - [x] RBAC は `config/eks-plugin/*.yaml` のみに寄せる

## DoD

- [ ] kind 上の BYO サンプルで、plugin を入れると `Cluster Available=True` まで到達できる（public endpoint + 到達性がある構成）
- [ ] plugin を入れない場合でも「kubectl 接続」の手順が明確（Available=False の理由も明確）
- [ ] Secret/credentials の責務が衝突せず、cleanup で AWS リソースが残らない
