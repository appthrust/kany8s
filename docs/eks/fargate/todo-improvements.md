# TODO: EKS Fargate bootstrap + Karpenter improvements

`docs/eks/fargate/` の MVP 実装（EKS BYO + Fargate bootstrap + Karpenter + Flux remote HelmRelease）を前提に、
運用性/削除の確実性/拡張性を上げるための改善バックログです。

## Cleanup / Deletion hardening
削除時に AWS リソースや Karpenter 起因の EC2 instance が残らないこと、kind を先に消して事故らないことを優先して強化します。

- [x] `eks-karpenter-bootstrapper` の delete 時の EC2 cleanup を best-effort から「取りこぼしにくい」設計へ寄せる（finalizer 導入 or 削除中の requeue/再試行方針を決める）
- [x] Cluster delete 開始時に Flux の `HelmRelease`/`OCIRepository` を `suspend`（または削除）して、Karpenter の再導入/置き戻しレースを抑止する
- [x] `hack/eks-fargate-dev-reset.sh` に「bootstrapper 管理リソースの delete 完了待ち」を追加する（OIDC/Role/Policy/InstanceProfile/AccessEntry/FargateProfile/Flux/CRS/ConfigMap/Secret）
- [x] `hack/eks-fargate-dev-reset.sh` に「AWS 側の消滅確認」を組み込む（`aws eks describe-cluster` 等）+ タイムアウト/リトライを明確化する
- [x] stuck finalizer / DeleteConflict / DependencyViolation の検知と見える化（ログ/表示）を dev reset に追加する
- [x] break-glass 手順（最後の手段）を docs として切り出す（デフォルトで実行しない）
- [x] bootstrapper が作成する AWS リソースへ AWS tags を付与し、棚卸し/手動復旧/コスト把握を容易にする（IAM/OIDC/FargateProfile/AccessEntry/SecurityGroup など）
- [x] node SecurityGroup deletion が orphan ENI で `DependencyViolation` になるケースを恒久対応する（delete finalizer で orphan ENI を削除して取りこぼしを減らす）

## Operability / Observability
steady-state 運用でのノイズを減らし、失敗の原因が 1 回の観測で分かるようにします。

- [x] `eks-kubeconfig-rotator` / `eks-karpenter-bootstrapper` の Event を state-change ベースに抑制し、定期 reconcile による Event ノイズを減らす
- [x] 両 controller のログに一貫したキーを追加する（`cluster`/`eksClusterName`/`ackClusterName`/`region`/`phase` など）
- [x] 代表的な failure mode を「原因 + 具体的対処」で Event に出す（private subnet 要件、NAT/VPC endpoints 不足、AccessEntry 前提の auth mode 不一致、Flux 未導入など）
- [x] 主要メトリクスを追加する（reconcile error、token 生成失敗、ownership conflict、最後に成功した同期時刻など）
- [x] Event の dedupe cache を bounded にする（TTL/size limit を導入し、長期稼働でメモリが増えないようにする）

## Performance / Scalability
複数クラスタ運用時の無駄な List/ポーリングを減らし、収束速度と負荷を改善します。

- [x] ACK EKS Cluster -> CAPI Cluster の mapping を namespace 内 List 依存から indexer ベースへ改善する（rotator/bootstrapper）
- [x] 派生リソース（FargateProfile/AccessEntry/Flux/CRS 等）を watch 対象に追加し、必要な status 変化で素早く再 reconcile する

## Configurability / Extensibility
「サンプル」から「検証/運用」へ寄せるため、固定値を減らして上書き手段を用意します。

- [x] Karpenter chart version を固定値から可変にする（flag/annotation/Topology variable で override）
- [x] HelmRelease values の上書き手段を提供する（ConfigMap/annotation 等の入力ソース + 安全な merge ルール）
- [x] NodePool/EC2NodeClass の “デフォルトテンプレ” を差し替え可能にする（per-cluster テンプレ参照 or inline YAML）
- [x] `vpc-security-group-ids` の多義性を減らすため、node 用 SG IDs を別変数として分離する案を検討する
- [x] node role の追加 managed policy（例: `AmazonSSMManagedInstanceCore`）を opt-in で付与できるようにする

## Validation / Safety
失敗を「作った後に気付く」から「作る前/早期に気付く」へ寄せ、誤操作・誤課金を減らします。

- [x] subnetIDs の事前検証を強化する（private/public、AZ 分散、同一 VPC などを DescribeSubnets で確認し、明確にエラー表示）
- [x] 前提 CRD/Controller 不足（ACK/Flux/CRS 等）を早期に検知し、requeue とメッセージを整理する
- [x] unmanaged resource/secret を上書きしない方針の “takeover UX” を docs と実装で統一する（削除方式、annotation による明示許可など）
- [x] unstructured の mutate helper（`mustSetNestedField` 等）の panic を避ける（既存 object 形状が想定外でも controller が落ちずにエラー扱いで継続できるようにする）
- [x] OIDC thumbprint auto の算出を VerifiedChains ベースにする（PeerCertificates の末尾を root 扱いしない）。root が送られない/検証できないケースの挙動（skip + Event など）も整理する
- [x] OIDC thumbprint の値を AWS IAM の要件（top intermediate CA thumbprint）に合わせる（VerifiedChains の最後=root ではなく、基本は chain[len-2] を使う）。docs の算出手順（JWKS endpoint 前提など）とテストも更新する

## Credentials / Deployment
kind と実クラスタで credentials 戦略が異なる前提を吸収し、導入の一貫性を高めます。

- [x] kind 向け（`ack-system/aws-creds` mount）と実クラスタ向け（IRSA）で kustomize overlay を用意し、どちらでも同じ手順でデプロイできるようにする
- [x] `aws-creds` の namespace/secret 名/パスの前提を docs と manifests で固定し、ドリフトを防ぐ
- [x] （任意）2つの EKS plugin を 1 deployment/binary に統合するか方針決定し、RBAC/責務/運用面のトレードオフを整理する

## Docs / UX
EKS nodeless（Fargate + Karpenter）の “使い始め/壊れた時/消す時” の迷いを減らします。

- [x] `examples/eks/README.md` に `<cluster>-kubeconfig-exec` の利用を追記し、人間用 kubeconfig と probe 用 kubeconfig の扱いを明確化する
- [x] BYO と bootstrap-network の削除範囲をマトリクス化して docs に追記する（EKS/IAM/Fargate/Flux/SG/EC2 vs VPC/Subnet）
- [x] plugin-managed リソースの観測コマンド集を 1 箇所にまとめる（label/annotation/tag を前提にして検索しやすくする）
- [x] ClusterClass の naming template 導入を検討し、Topology のランダム suffix を抑止して観測/名前解決を単純化する
- [x] `examples/eks/manifests/clusterclass-eks-byo.yaml` を docs と同期する（`vpc-node-security-group-ids` / `karpenter-node-role-additional-policy-arns` など、Fargate+Karpenter で使う変数を反映）
- [x] `docs/eks/fargate/design.md` の security group 変数説明を更新する（`vpc-node-security-group-ids` 優先 + 後方互換として `vpc-security-group-ids` を利用する旨を明記）

## Testing / Quality
回帰しやすいロジック（unstructured 生成/削除/ゲート/競合）を自動テストで固めます。

- [x] envtest を追加して、ACK EKS status（unstructured）から派生リソースが生成されること/ready gate が機能することを検証する（bootstrapper）
- [x] envtest を追加して、region/name 解決・ownership conflict・requeue policy が期待通りであることを検証する（rotator）
- [x] “BYO + plugins + opt-in => node join” の手順をスクリプト化し、acceptance/e2e の再現性を上げる
- [x] Flux components の version を pin し、upgrade 手順（互換性/破壊的変更）を docs 化する

## Future / Optional
MVP の外だが、必要になりがちな拡張です。

- [x] interruption handling をオプションで追加する（現スコープ: `settings.interruptionQueue` 注入。SQS/EventBridge リソース作成は対象外）
- [x] OIDC thumbprint を自動算出して `OpenIDConnectProvider.spec.thumbprints` を明示設定する（より厳密なセキュリティ寄り構成）
- [x] interruption handling のスコープを明確化する（`settings.interruptionQueue` 注入のみ / SQS+EventBridge まで含める）。後者なら controller role への SQS 権限追加 + 手順/前提の docs を追加する
