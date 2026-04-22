# EKS Plugins

`docs/eks/` 配下の EKS 手順/サンプルを「運用/検証に使える」状態に近づけるための、EKS 専用プラグイン設計メモを置くディレクトリです。

Kany8s 自体は provider-agnostic を志向し、ControlPlane の kubeconfig 生成/取得も標準化した Secret 形へ寄せています。
一方で EKS のように認証が IAM token 依存で短命な環境では、CAPI の `RemoteConnectionProbe`（ひいては `Cluster Available=True`）を安定させるには追加コンポーネントが必要になることがあります。

## Install (Helm)

`clusterctl` / cluster-api-operator は CAPI provider manager のみを install する。
EKS 固有の plugin は **別途 Helm で入れる**。GHCR に OCI 形式で publish されている。

```bash
# EKS kubeconfig rotator (必須: 短命 token を CAPI kubeconfig Secret に反映)
helm install rotator oci://ghcr.io/appthrust/charts/eks-kubeconfig-rotator \
  --version <tag-without-v> \
  --namespace kany8s-eks-system --create-namespace \
  --set aws.mode=irsa \
  --set aws.irsa.roleArn=arn:aws:iam::123456789012:role/eks-rotator

# EKS Karpenter bootstrapper (任意: Karpenter を使う場合のみ)
helm install bootstrapper oci://ghcr.io/appthrust/charts/eks-karpenter-bootstrapper \
  --version <tag-without-v> \
  --namespace kany8s-eks-system \
  --set aws.mode=irsa \
  --set aws.irsa.roleArn=arn:aws:iam::123456789012:role/eks-karpenter-bootstrapper
```

- `<tag-without-v>` は kany8s のリリースタグから先頭 `v` を除いた SemVer（例: `0.1.1`）。
- `aws.mode` は `staticSecret | irsa | podIdentity` の 3 種。
- 全 values と上書きレシピは `charts/eks-kubeconfig-rotator/README.md` / `charts/eks-karpenter-bootstrapper/README.md` を参照。

従来の `config/eks-plugin/` / `config/eks-karpenter-bootstrapper/` kustomize overlay はそのまま残してあるが、
新規導入では Helm chart を推奨する。

## Documents

- `docs/eks/plugin/eks-kubeconfig-rotator.md`
  - EKS 用に CAPI 準拠 kubeconfig を作り、短命 token をローテーションして `RemoteConnectionProbe` を通すためのプラグイン設計。

- `docs/eks/plugin/todo.md`
  - 上記設計を実装に落とすための TODO。

- `docs/eks/plugin/test.md`
  - `eks-kubeconfig-rotator` の動作確認手順（kind + BYO サンプル）。
