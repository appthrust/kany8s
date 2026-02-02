# PRD Details: Infrastructure Policy (Approach A)

- 作成日: 2026-02-02
- 対象: `docs/PRD.md` の Infrastructure (`Kany8sCluster`) 方針の補足

このドキュメントは、`docs/PRD.md` に記載した infra 側の新方針（案A寄せ）の「なぜそうするのか」「どの案を比較してどう判断したか」を残すための詳細メモです。

## 1. 変更点（要約）

`Kany8sCluster` を kro(RGD) と連携させる拡張は継続して検討しますが、**infra の出力（例: VPC ID 等）を `Kany8sCluster` から control plane へ受け渡す仕組みは導入しない**方針に寄せます。

代わりに、infra と control plane の間で値の受け渡しが必要な場合は、**`Kany8sControlPlane` が参照する "親 RGD" の中に infra も含め、RGD chaining/DAG 内で完結**させます（案A）。

## 2. なぜ案Aに寄せるのか（背景と判断）

Kany8s の中核は「CAPI contract を満たす薄い provider」と「具象化は RGD へ委譲」を両立させることです。

infra を別コンポーネント（`Kany8sCluster`）として kro で具象化し、その outputs を control plane に渡す設計は、一見すると責務分離に見えます。しかし現実には次の問題が発生しやすく、Kany8s の原則（provider-agnostic / Secrets 最小 / controller を薄く保つ）と衝突します。

- **outputs の一般化圧**: VPC ID、Subnet IDs、SecurityGroup IDs、LB hostname など、クラウドごとに形が違う値を「後段に渡す」必要が出ると、最終的に "汎用 outputs"（Terraform outputs のような仕組み）へ収束しやすい。
- **controller 間の結合増大**: `Kany8sCluster` が出す値を `Kany8sControlPlane` が読むようにすると、CRD 設計・バージョニング・エラーハンドリング・同期順序が一気に複雑になり、"薄い provider" から外れやすい。
- **同期順序の難しさ**: CAPI の reconcile は非同期で進むため、「infra 完了→control plane 開始」の順序保証を controller 間の参照で作ると待機/リトライの設計が重くなりがち。
- **kro の落とし穴の影響が大きい**: kro v0.7.1 では status field 欠落（特に bool）等が起こり得るため、infra 側の gate（`provisioned`）を通す設計は "詰まり" を引き起こしやすい。

案A（親 RGD に寄せて閉じる）なら、依存関係の順序と値の受け渡しを kro の DAG/chaining に閉じ込められます。
Kany8s controller は引き続き「kro instance の正規化 status を読む」だけに集中でき、拡張の中心は RGD になります。

## 3. 検討した選択肢（A/B/C）

### A: 依存関係がある範囲は "親 RGD" に寄せる（採用）

- 何をする: infra と control plane を同じ kro instance の graph（親 RGD）に含めて chaining し、値の受け渡しは RGD 内参照で行う。
- 良い点:
  - outputs の一般化が不要（Secret/汎用 outputs をコアに持ち込まない）
  - controller 実装が薄いまま（provider-agnostic を維持）
  - 依存順序が RGD/DAG に閉じる
- 弱い点:
  - 「infra と control plane を完全に別ライフサイクルにしたい」ケースでは、RGD 分割/運用設計が必要

### B: 最小の typed outputs を `Kany8sCluster.status` に持つ

- 何をする: provider-agnostic な形に限定して outputs を CRD に追加し、control plane が参照する。
- 良い点: Secret に汎用 outputs を作らずに済む。
- 弱い点: Kany8s の API surface/互換性コストが増え、結局 "どこまでを標準化するか" の議論が続く。

### C: Secret/ConfigMap で outputs を受け渡す

- 何をする: CAPT 的な outputs パターンを限定復活させる。
- 良い点: 実装は分かりやすい。
- 弱い点: Kany8s の非ゴール（outputs をコアにしない）に反し、運用複雑性が上がる。

## 4. 具体的な運用イメージ（案Aの推奨パターン）

### 4.1 managed control plane（推奨）

- `Kany8sControlPlane` が参照する RGD を "親 RGD" とし、その中で:
  - 必要な infra 前提（例: IAM Role、VPC 関連など）
  - managed control plane（例: EKS/GKE/AKS）
  を chaining/DAG で合成する。
- Kany8s controller は親 RGD instance の `status.ready/status.endpoint/...` のみを読む。
- 再利用したい VPC 等がある場合は outputs で渡さず、`Kany8sControlPlane.spec.kroSpec`（または Topology variables）で入力として渡す。

### 4.2 `Kany8sCluster` kro mode（位置付け）

`Kany8sCluster` の kro mode は、"infra を kro で具象化し、InfrastructureCluster contract を満たす" ための拡張として有用ですが、
**control plane へ値を配るための仕組みにはしない**（案A）という位置付けになります。

## 5. PRD 側に残すべきルール（要点）

- infra と control plane の値受け渡しは、Kany8s の CR 間で行わず、RGD 内で閉じる（親 RGD / chaining / DAG）。
- `Kany8sCluster` は provider-specific CR を直接読まず、kro instance の正規化 status のみを読む。
- "汎用 outputs" をコア設計として採用しない（例外を作る場合は別途 ADR 的に明文化する）。
