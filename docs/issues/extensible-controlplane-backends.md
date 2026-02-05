# Issue: Self-Managed ControlPlane backends をユーザが拡張できる設計

- 作成日: 2026-02-05
- ステータス: Open (design needed)

## 背景

Kany8s は `Kany8sCluster` (Infrastructure) + `Kany8sControlPlane` (ControlPlane) という Cluster API-facing な薄い provider suite を提供し、provider-specific な具象化は kro RGD に委譲する方針を採っています。

一方で、managed control plane (EKS/GKE/AKS 等) と異なり、self-managed control plane は「外部が control plane を提供してくれない」ため、
Machine/Bootstrap/証明書/kubeconfig 等を含む ControlPlane provider 相当の controller 実装が必要になります。

現状は self-managed 向けに `Kany8sKubeadmControlPlane` が存在し、kubeadm を前提とした controller 実装を Kany8s 本体が持っています。

## 問題

クラスタ作成の backend が kubeadm 以外（例: Talos/k0s/RKE2/独自 OS/独自オンプレ基盤）でも self-managed control plane を作りたい場合、
Kany8s 本体に実装を追加するか fork する必要が出やすい。

ここはユーザが自分の環境に合わせて controller を実装できるように、Kany8s の設計として「拡張可能な ControlPlane backend」を提供する必要がある。

## ゴール

- ユーザが out-of-tree の controller を実装し、Kany8s を fork せずに self-managed control plane の backend を追加できる
- `Kany8sControlPlane` は provider-agnostic の境界を維持し、backend 固有 CR の shape を読まず、正規化された status contract のみで CAPI contract を満たす
- 既存の managed(kro) / self-managed(kubeadm) の道筋と矛盾しない（移行可能、または並存可能）

## Non-goals

- 「どの self-managed backend が正しいか」を決めること自体（ここでは拡張機構の設計が目的）
- 各 backend の詳細実装（Talos など）をこの issue で実装すること

## 設計上の論点

- Extension point をどこに置くか
  - `Kany8sControlPlane` を facade として複数 backend に委譲するのか
  - それとも self-managed は素直に別 ControlPlane provider を使う（= `Kany8sControlPlane` は kro 専用）と割り切るのか

- Backend の contract
  - 最低限必要な status（`ready/endpoint/reason/message/kubeconfigSecretRef`）は `docs/adr/0002-normalized-rgd-instance-status-contract.md` と整合させられるか
  - kubeconfig Secret の責務分担（Option A/B）は `docs/adr/0004-kubeconfig-secret-strategy.md` と整合させられるか

- Ownership / Lifecycle
  - backend CR の作成/更新/削除を誰が行うか（`Kany8sControlPlane` が作る vs ユーザが作る）
  - 1:1 リソースモデル（ControlPlane 1つにつき backend 1つ）を維持するか

- Watch/RBAC
  - 動的 GVK を扱う場合の RBAC（`kro.run resources=*` と同様の tradeoff）
  - 既存の dynamic watch 機構との整合

- ClusterClass/Topology
  - Template から backend を選択する入力面（`Kany8sControlPlaneTemplate` とどう噛み合わせるか）

## 方向性（候補）

### Option A: `Kany8sControlPlane` を facade にし、backend CR を参照/委譲する

- `Kany8sControlPlane.spec` に backend 選択（例: `backendRef`）を持たせる
- Kany8s は backend CR の status を正規化 contract として読み、CAPI contract を満たすための反映（endpoint/initialized/conditions/kubeconfig）だけを担当する
- managed(kro) は backend=RGD instance として扱える
- self-managed(kubeadm) は backend=`Kany8sKubeadmControlPlane` を builtin backend として扱える
- ユーザは独自 backend CRD/controller を追加し、同じ status contract を満たすことで拡張できる

懸念:

- backendRef が dynamic GVK になり RBAC/Watch が難しくなる
- 既存 API の後方互換/移行設計が必要

### Option B: `Kany8sControlPlane` は kro 専用にし、self-managed は別 provider を使う

- self-managed で拡張したいユーザは CAPI の別 ControlPlane provider（もしくは自作）を使う
- Kany8s の責務が増えずシンプル

懸念:

- 「`Kany8sCluster` + `Kany8sControlPlane` だけ apply したい」UX（統一された入口）が崩れる
- kubeconfig Secret 戦略や status 正規化を Kany8s 側で統一しにくい

## 成果物（この issue の DoD）

- 以下を ADR として確定する
  - ControlPlane backend 拡張方針（Option A/B の選定）
  - backend status contract（`docs/adr/0002-...` の再利用 or 拡張）
  - kubeconfig Secret の責務分担（`docs/adr/0004-...` との整合）

- API/Docs の更新方針が決まっている
  - `Kany8sControlPlane` の新 spec フィールド（必要なら）
  - Template/Topology での指定方法（必要なら）
  - RGD authors / backend authors 向けのガイド

## 参考

- PRD: `docs/PRD.md`
- ADR index: `docs/adr/README.md`
- provider-agnostic 方針: `docs/adr/0001-provider-agnostic-kany8s-via-kro.md`
- 正規化 status contract: `docs/adr/0002-normalized-rgd-instance-status-contract.md`
- kubeconfig Secret 方針: `docs/adr/0004-kubeconfig-secret-strategy.md`
- dynamic GVK RBAC tradeoff: `docs/adr/0007-dynamic-gvk-rbac-tradeoffs.md`
- self-managed の境界: `docs/adr/0009-self-managed-kubeadm-boundaries.md`
