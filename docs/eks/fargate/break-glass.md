# Break-glass: EKS Fargate cleanup（最後の手段）

この手順は通常運用では使わないでください。  
まず `hack/eks-fargate-dev-reset.sh` を実行し、それでも削除が進まない場合のみ実施します。

## 1) 状況確認

```bash
export NAMESPACE=default
export CLUSTER_NAME=<cluster-name>
export AWS_REGION=<region>
export EKS_CLUSTER_NAME=<eks-cluster-name>

kubectl -n "$NAMESPACE" get \
  openidconnectproviders.iam.services.k8s.aws,roles.iam.services.k8s.aws,policies.iam.services.k8s.aws,instanceprofiles.iam.services.k8s.aws,\
  accessentries.eks.services.k8s.aws,fargateprofiles.eks.services.k8s.aws,securitygroups.ec2.services.k8s.aws,\
  ocirepositories.source.toolkit.fluxcd.io,helmreleases.helm.toolkit.fluxcd.io,\
  configmaps,secrets,clusterresourcesets.addons.cluster.x-k8s.io \
  -l cluster.x-k8s.io/cluster-name="$CLUSTER_NAME" -o wide
```

## 2) Kubernetes 側の詰まりを解消

```bash
# finalizer が詰まっている resource を確認
kubectl -n "$NAMESPACE" get <kind>/<name> -o jsonpath='{.metadata.finalizers}'

# 明示削除（必要に応じて）
kubectl -n "$NAMESPACE" delete <kind> <name> --wait=false
```

必要な調査:

- `kubectl -n "$NAMESPACE" describe <kind> <name>` で `DeleteConflict` / `DependencyViolation` を確認
- ACK controller logs を確認（`ack-system` namespace）

## 3) AWS 側の残骸を削除

```bash
# EKS cluster
aws eks delete-cluster --region "$AWS_REGION" --name "$EKS_CLUSTER_NAME"

# Karpenter EC2 instances（discovery tag）
INSTANCE_IDS="$(aws ec2 describe-instances \
  --region "$AWS_REGION" \
  --filters "Name=tag:karpenter.sh/discovery,Values=$EKS_CLUSTER_NAME" \
            "Name=instance-state-name,Values=pending,running,stopping,stopped,shutting-down" \
  --query 'Reservations[].Instances[].InstanceId' --output text)"

if [[ -n "$INSTANCE_IDS" ]]; then
  aws ec2 terminate-instances --region "$AWS_REGION" --instance-ids $INSTANCE_IDS
fi

# (任意) node SecurityGroup deletion が DependencyViolation で詰まる場合の orphan ENI cleanup
# - ACK SecurityGroup CR の status.id を参照して SG_ID を取得してから実行
# - Attachment が null の ENI のみを対象にする（アタッチ済み ENI を消すと通信断になります）
SG_ID=<sg-id>
ORPHAN_ENIS="$(aws ec2 describe-network-interfaces \
  --region "$AWS_REGION" \
  --filters "Name=group-id,Values=$SG_ID" "Name=tag:eks:eni:owner,Values=amazon-vpc-cni" \
  --query "NetworkInterfaces[?Attachment==\`null\` && Status=='available'].NetworkInterfaceId" \
  --output text)"

if [[ -n "$ORPHAN_ENIS" ]]; then
  aws ec2 delete-network-interface --region "$AWS_REGION" --network-interface-id $ORPHAN_ENIS
fi
```

## 4) 再確認

```bash
aws eks describe-cluster --region "$AWS_REGION" --name "$EKS_CLUSTER_NAME" || true
```

`ResourceNotFoundException` が返れば削除完了です。
