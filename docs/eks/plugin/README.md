# EKS Plugins

`docs/eks/` 配下の EKS 手順/サンプルを「運用/検証に使える」状態に近づけるための、EKS 専用プラグイン設計メモを置くディレクトリです。

Kany8s 自体は provider-agnostic を志向し、ControlPlane の kubeconfig 生成/取得も標準化した Secret 形へ寄せています。
一方で EKS のように認証が IAM token 依存で短命な環境では、CAPI の `RemoteConnectionProbe`（ひいては `Cluster Available=True`）を安定させるには追加コンポーネントが必要になることがあります。

## Documents

- `docs/eks/plugin/eks-kubeconfig-rotator.md`
  - EKS 用に CAPI 準拠 kubeconfig を作り、短命 token をローテーションして `RemoteConnectionProbe` を通すためのプラグイン設計。

- `docs/eks/plugin/todo.md`
  - 上記設計を実装に落とすための TODO。

- `docs/eks/plugin/test.md`
  - `eks-kubeconfig-rotator` の動作確認手順（kind + BYO サンプル）。
