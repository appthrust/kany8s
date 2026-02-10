# Flux version pin / upgrade runbook

`docs/eks/fargate/` の手順では Flux を pin して導入します。

- pin: `v2.4.0`
- install script: `hack/eks-install-flux.sh`

## Install (pinned)

```bash
export FLUX_VERSION=v2.4.0
bash hack/eks-install-flux.sh
```

## Upgrade procedure

1. 管理クラスタで現在バージョンを確認

```bash
kubectl -n flux-system get deploy/source-controller deploy/helm-controller \
  -o jsonpath='{range .items[*]}{.metadata.name}{" => "}{.spec.template.spec.containers[0].image}{"\n"}{end}'
```

2. 目標バージョンを設定して apply

```bash
export FLUX_VERSION=v2.4.1
bash hack/eks-install-flux.sh
```

3. API 互換を確認（bootstrapper 依存）

```bash
kubectl api-resources --api-group=source.toolkit.fluxcd.io | rg -i ocirepository
kubectl api-resources --api-group=helm.toolkit.fluxcd.io | rg -i helmrelease
```

4. 既存クラスタの Karpenter HelmRelease が維持されることを確認

```bash
kubectl -n "$NAMESPACE" get helmreleases.helm.toolkit.fluxcd.io -l cluster.x-k8s.io/cluster-name="$CLUSTER_NAME"
kubectl -n "$NAMESPACE" describe helmrelease "${CLUSTER_NAME}-karpenter"
```

## Rollback

互換性問題が出た場合は `FLUX_VERSION` を直前バージョンへ戻して同じスクリプトを再実行します。
