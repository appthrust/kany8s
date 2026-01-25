# Kany8s Design (Draft)

## 0. このドキュメントの目的

本ドキュメントは、以下を明確化するための設計メモです。

- Kro を「具象化エンジン」として利用し、Cluster API (CAPI) と連携してマネージド Kubernetes を作る Provider を実現する
- その Provider を将来的にマルチクラウドへ拡張する前提で、コントローラ側の責務と RGD 側の責務を切り分ける
- ここまでの議論で確定した判断（採用/非採用）を明確に残す

## 1. 背景

- 既存の CAPT (Cluster API Terraform) では、Terraform Workspace を「実行単位/状態単位」として扱い、
  その outputs を Secret へ書き出し、依存関係を WorkspaceTemplateApply で制御していました。
- Kany8s では Terraform を前提にせず、Kro + 各クラウド Provider (当面は ACK) で具象リソースを作る構成へ寄せます。

## 2. ゴール / 非ゴール

### 2.1 ゴール

- CAPI 準拠の ControlPlane Provider を実装する（endpoint/initialized/conditions を CAPI contract の期待通りに埋める）
- Kany8s コントローラにクラウド固有の処理（EKS 依存の status 読み取り等）を書かない
- Kro の RGD を部品化しつつ、少ない数の Kro インスタンスで ACK CR 群を管理できるようにする

### 2.2 非ゴール（現時点でやらないこと）

- CAPT の WorkspaceTemplate → WorkspaceTemplateApply の仕組みを Kro + ACK で再現しない
- outputs を Secret に書き出して後段へ渡す、という設計を Kany8s の中核にはしない
  - （将来的に必要になれば追加は可能だが MVP の必須要件ではない）

## 3. 重要な設計判断（決定事項）

### 3.1 Kany8s コントローラは「kro instance の status だけ」を参照する

**Decision**
- Kany8s コントローラは、ACK などクラウド固有の CR の status を直接読まない。
- Kany8s が参照するのは、kro の RGD インスタンス（以下 kro instance）の `status` のみとする。

**Rationale**
- Kany8s が EKS/ACK の status 形式に依存すると、今後 AKS/GKE 等を追加するたびに Kany8s 側へ分岐実装が増える。
- provider 固有差分は kro 側（RGD）で吸収し、Kany8s を provider-agnostic に保つ方が拡張性が高い。

**Consequence**
- kro instance の status に「Kany8s が期待する正規化フィールド」を用意する必要がある。

### 3.2 CAPT の “Template→Apply” は Kany8s では中核にしない

**Decision**
- CAPT の WorkspaceTemplate/Apply パターン（テンプレと適用の分離、依存待ち、変数差し替え）は Kany8s のコア機能としては再現しない。

**Rationale**
- Kro には DAG による依存解決と合成があり、Terraform Workspace の “Apply 単位管理” をそのまま持ち込む必要性が薄い。
- Kany8s の焦点は「CAPI の Provider contract を満たすこと」と「kro で具象化すること」。

### 3.3 outputs を Secret に書かない（当面）

**Decision**
- endpoint/initialized/conditions のために outputs を Secret に書き出す設計は採用しない。

**Rationale**
- Kany8s は kro instance の `status` を直接消費できるため、ControlPlane endpoint/initialized 判定のために “汎用 outputs” を Secret へ書き出す必要はない。
- ただし `*-kubeconfig` Secret（Cluster API contract の kubeconfig management）は別要件であり、CAPI 準拠のためには実装が必要になる。

### 3.4 CAPI 準拠のため endpoint/initialized/conditions は contract に従って実装する

**Decision**
- Kany8s は ControlPlane provider として、少なくとも以下を満たす。
  - endpoint provider として `Kany8sControlPlane.spec.controlPlaneEndpoint`（host/port）を設定する
  - `Kany8sControlPlane.status.initialization.controlPlaneInitialized` を設定する
  - `Kany8sControlPlane.status.conditions`（および必要な status フィールド）を更新する
- `Kany8sControlPlane.spec.controlPlaneEndpoint` と `status.initialization.controlPlaneInitialized` が揃った後、Cluster API の Cluster controller が contract に従って `Cluster.spec.controlPlaneEndpoint` を反映するため、Kany8s は `Cluster.spec.controlPlaneEndpoint` を直接 patch しない

**Rationale**
- Cluster API の ControlPlane contract は、endpoint と initialization 完了の “置き場所” を定義している。
- contract に従うことで、Cluster API の internal 実装に依存せずに連携できる。

### 3.5 ControlPlane が使う RGD は `resourceGraphDefinitionRef` で選択する

**Decision**
- Kany8s ControlPlane CR は、使用する kro の `ResourceGraphDefinition` を `spec.resourceGraphDefinitionRef` で参照する。
- Kany8s コントローラは参照された RGD を読み、RGD が生成する Custom API の GVK を解決して、その instance を作成・監視する。

**Rationale**
- provider 追加（AKS/GKE 等）のたびに Kany8s コントローラへ分岐を増やさず、RGD の追加・差し替えだけで対応できる。
- CAPT の `WorkspaceTemplateRef` と同様に「テンプレ参照を spec に持つ」形は運用上理解しやすい。

### 3.6 ControlPlane RGD のスコープは「ControlPlane と前提」まで（MVP）

**Decision**
- Kany8s が参照する RGD は、まず **ControlPlane と、その作成に必要な前提（例: 必須 IAM Role 等）**のみに責務を絞る。
- Addon / PodIdentity / S3 / SQS / EventBridge など “クラスタ利用後の周辺” は **別の RGD/別のオーケストレーション**に分離する。

**Rationale**
- CAPI の actuation の中核は `ControlPlaneRef` と `InfrastructureRef`。
- ControlPlane provider の `Ready` を周辺構築まで引きずると、CAPI 側の期待（「ControlPlane ready」=「API endpoint を設定できる」）とズレやすい。

## 4. アーキテクチャ概要

### 4.1 コンポーネント

- **Kany8s ControlPlane CRD**
  - Cluster API の ControlPlane provider contract を満たすための CR（例: `Kany8sControlPlane`）。

- **kro instance**
  - RGD によって定義される “正規化された status を持つ” インスタンス。
  - プロバイダ固有の ACK CR 群などを内包して生成・更新する。

- **プロバイダ具象リソース**
  - 当面は ACK (EKS/IAM/S3/SQS/EventBridge) を想定。
  - 将来的に AKS/GKE 等へ拡張。

### 4.2 Provider 非依存の境界（重要）

- Kany8s コントローラは `kro instance.status` の “正規化情報” だけを消費する。
- ACK/EKS の status フィールド差異、Ready 判定、endpoint 抽出などは RGD 側で吸収する。

## 5. kro instance status の「正規化インターフェース」

Kany8s が参照する kro instance（ControlPlane RGD instance）の最小契約を定義する。

### 5.0 必須 spec フィールド（Kany8s → kro instance）

- `metadata.name: string`
  - **意味**: CAPI `Cluster` と同じ名前（= ControlPlane の識別子）
  - **方針**: ControlPlane RGD は `schema.metadata.name` をクラスタ名として利用する。

- `spec.version: string`
  - **意味**: Kubernetes version
  - **方針**: Kany8s コントローラが `ControlPlane.spec.version` を kro instance の `spec.version` に **必ず注入（上書き）**する。
  - **要求**: Kany8s 互換の ControlPlane RGD は `spec.version` を必須フィールドとして受け取り、provider 固有のリソースへ反映する。

### 5.1 必須 status フィールド（MVP）

- `status.ready: boolean`
  - **意味**: **ControlPlane ready**（CAPI の ControlPlane provider として Ready と言える状態）を表す。
  - **定義**: 少なくとも API endpoint が確定しており、`Kany8sControlPlane.spec.controlPlaneEndpoint` を設定できる。
  - **方針**: Addon/S3/SQS/EventBridge など周辺リソースの readiness は `status.platformReady` 等の別フィールドに分離し、`status.ready` には含めない。

- `status.endpoint: string`
  - **意味**: ControlPlane の API Server endpoint。
  - **形式**:
    - `https://host[:port]` もしくは `host[:port]`
  - Kany8s は URL として parse し、port 無指定の場合は `443` とする。

### 5.2 任意フィールド（推奨）

- `status.reason: string`
- `status.message: string`

目的は、CAPI 側の Condition の Reason/Message へ反映できるようにすること。

### 5.3 kro の予約フィールド

kro は全ての instance に対して、以下の `status` フィールドを自動で追加します。

- `status.conditions`（overall readiness 等の Conditions）
- `status.state`（`ACTIVE` / `IN_PROGRESS` / `FAILED` / `DELETING` / `ERROR`）

これらは kro 側で上書きされるため、Kany8s 用の正規化フィールドは `ready/endpoint/reason/message` のような別名で定義します。

## 6. Kany8s ControlPlane コントローラのリコンシリエーション（概略）

### 6.1 Create/Update

1. `Kany8sControlPlane` を受け取る
2. 対応する kro instance を作成/更新する（OwnerReference を付与、`spec.version` は `Kany8sControlPlane.spec.version` で上書き）
3. kro instance の status を監視する
4. kro instance がまだ Ready でない間:
   - ControlPlane: `Creating` などの conditions/status を更新
   - `Kany8sControlPlane.spec.controlPlaneEndpoint` は未設定のまま
   - `Kany8sControlPlane.status.initialization.controlPlaneInitialized` は未設定（または false）
5. kro instance が Ready + endpoint を返したら:
   - endpoint（string）を parse し `Kany8sControlPlane.spec.controlPlaneEndpoint`（host/port）を設定
   - `Kany8sControlPlane.status.initialization.controlPlaneInitialized=true` を設定
   - ControlPlane: `Ready` / `Available` を True にして status/conditions を更新
   - Cluster API の Cluster controller が contract に従って `Cluster.spec.controlPlaneEndpoint` を反映する

### 6.2 Delete

- kro instance は OwnerReference により削除連鎖させる。
- `Cluster.spec.controlPlaneEndpoint` の変更は Cluster controller の責務なので、Kany8s 側では削除時に直接クリアしない。

### 6.3 Kany8s ControlPlane CRD（案）

Kany8s の ControlPlane CR は、`resourceGraphDefinitionRef` で選んだ RGD が生成する Custom API の **instance を 1:1 で管理**します。

#### Spec（案）

- `spec.version: string`（required）
  - Kubernetes version（CAPI と同じ）
  - Kany8s はこの値を kro instance の `spec.version` に注入（上書き）する

- `spec.resourceGraphDefinitionRef.name: string`（required）
  - 参照する `ResourceGraphDefinition` 名（RGD は cluster-scoped）

- `spec.kroSpec: object`（optional）
  - RGD instance の `.spec` として渡す provider-specific パラメータ（`version` 以外）
  - 形は RGD に依存するため、CRD は厳密なスキーマ検証を行わない

- `spec.controlPlaneEndpoint: APIEndpoint`（optional）
  - endpoint provider として controller が書き込む
  - `kro instance.status.endpoint`（string）を URL parse して host/port を設定する

#### Status（案）

- `status.ready: boolean`
  - kro instance の `status.ready` を反映（運用上の簡易フラグ）

- `status.version: string`
  - ControlPlane の実バージョン（基本は `spec.version` と一致）

- `status.externalManagedControlPlane: *bool`
  - managed control plane（EKS/GKE/AKS 等）の場合は true

- `status.initialization.controlPlaneInitialized: *bool`
  - ControlPlane が API を受け付けられる状態になったら true

- `status.failureReason` / `status.failureMessage`
  - kro instance の `status.reason/message` などを反映（存在する場合）

- `status.conditions: []metav1.Condition`
  - CAPI 慣習に合わせた Conditions（Ready/Creating/Failed など）

#### ControlPlane の YAML イメージ

```yaml
apiVersion: controlplane.cluster.x-k8s.io/v1alpha1
kind: Kany8sControlPlane
metadata:
  name: demo-cluster
  namespace: default
spec:
  version: "1.34"
  resourceGraphDefinitionRef:
    name: eks-control-plane
  kroSpec:
    region: ap-northeast-1
    vpc:
      subnetIDs:
        - subnet-xxxx
        - subnet-yyyy
      securityGroupIDs:
        - sg-zzzz
```

Kany8s controller は上記から、次の kro instance を作成します（GVK は RGD の schema から解決）。

```yaml
apiVersion: kro.run/v1alpha1
kind: EKSControlPlane
metadata:
  name: demo-cluster
  namespace: default
spec:
  version: "1.34" # injected
  region: ap-northeast-1
  vpc:
    subnetIDs:
      - subnet-xxxx
      - subnet-yyyy
    securityGroupIDs:
      - sg-zzzz
```

## 7. RGD の分割方針（ACK/EKS を例に）

将来のマルチクラウド化を見据え、「巨大な 1 枚 RGD」ではなく部品化し、親 RGD で束ねる。

### 7.1 MVP（ControlPlane）

Kany8s の ControlPlane provider が参照するのは、まず以下の **ControlPlane 専用 RGD**のみとする。

- `EKSControlPlane`（ACK EKS Cluster + ControlPlane の前提）

ここでの `status.ready` は ControlPlane ready の意味に固定する（`design.md` の 5.1 参照）。

### 7.2 周辺リソース（非MVP、別RGD）

`idea.md` の A〜G のうち、周辺（ControlPlane 以外）は別 RGD として切り出し、必要に応じて後段で chaining する。

- `EKSAddons`（ACK Addon 群）
- `PodIdentitySet`（ACK PodIdentityAssociation 群 + その前提 IAM Role 群）
- `ObservabilityBuckets`（ACK S3 Bucket x3）
- `KarpenterInterrupt`（ACK SQS Queue + EventBridge Rule 群）

### 7.3 親 RGD（例: `PlatformCluster`）

- 上記部品 RGD を chaining で呼び出し、入力を配布する
- 親 RGD の `status` は用途に応じて分ける
  - 例: `status.controlPlaneReady` / `status.controlPlaneEndpoint` / `status.platformReady`
- Kany8s（ControlPlane provider）が参照するのは ControlPlane 専用 RGD（例: `EKSControlPlane`）を基本とする
  - ただし将来的に Kany8s が「platform orchestration」まで担う場合は別途検討する

### 7.4 依存関係制御

- RGD 内では Kro の DAG に依存解決を任せる。
- 依存順序は「明示 dependsOn」よりも、テンプレ内参照（`${...}`）で自然に DAG が張られる形を優先する。
  - 例: Role の ARN を Cluster spec に埋め込む形にして、Role → Cluster の順序を強制する

## 8. kro status 正規化の実装（ACK EKS 例）

このセクションは「Kany8s コントローラが参照する `kro instance.status` を、RGD 側でどう作るか」を具体例で示します。

### 8.1 前提: `schema.status` と `readyWhen`

- kro の RGD では `spec.schema.status` に CEL 式 `${...}` を書くことで、子リソースの status を instance の status に持ち上げられます。
- `readyWhen` を利用し、リソースが「ControlPlane ready 判定に必要な状態」になるまで待つようにできます。
  - 例えば ACK EKS Cluster なら `ACTIVE` + `endpoint != ""` など
  - 周辺リソース（Addon 等）まで同じ RGD に入れる場合も、同じ仕組みで依存制御できます（非MVP）。

### 8.2 ACK EKS Cluster からの正規化

ACK EKS Cluster（`eks.services.k8s.aws/v1alpha1` `Cluster`）は CRD 上、主に以下の status を持ちます。

- `.status.endpoint: string`（Kubernetes API server endpoint）
- `.status.status: string`（EKS cluster status。例: `ACTIVE`）
- `.status.conditions[]`（ACK conditions。例: `ACK.ResourceSynced`）

MVP の正規化は次を基本とします。

- `status.endpoint = ${cluster.status.endpoint}`
- `status.ready = ${cluster.status.status == "ACTIVE" && cluster.status.endpoint != "" && cluster.status.conditions.exists(c, c.type == "ACK.ResourceSynced" && c.status == "True")}`

また、EKS Cluster の作成には IAM Role が前提になります。RGD 内で Role を作る場合は、Cluster の `spec.roleARN` に **Role の ARN（例: `${clusterRole.status.ackResourceMetadata.arn}`）を参照**させ、Role が AWS 側で作成済みになるまで Cluster 作成を待ち合わせるのが安全です（次節の例参照）。

### 8.3 `EKSControlPlane` RGD（抜粋）

```yaml
apiVersion: kro.run/v1alpha1
kind: ResourceGraphDefinition
metadata:
  name: eks-control-plane
spec:
  schema:
    apiVersion: kro.run/v1alpha1
    kind: EKSControlPlane
    spec:
      # 推奨: instance の metadata.name を cluster 名として使う
      # （Kany8s 側で instance 名を決め打ちでき、spec の重複を避けられる）
      region: string | required=true
      version: string | required=true

      # EKS Cluster IAM Role（MVPでは AWS managed policy を attach する）
      clusterRolePolicies: "[]string" | default=["arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"]

      vpc:
        subnetIDs: "[]string" | required=true
        securityGroupIDs: "[]string" | required=true
        endpointPrivateAccess: boolean | default=true
        endpointPublicAccess: boolean | default=true
        publicAccessCIDRs: "[]string" | default=["0.0.0.0/0"]
    status:
      ready: ${cluster.status.status == "ACTIVE" && cluster.status.endpoint != "" && cluster.status.conditions.exists(c, c.type == "ACK.ResourceSynced" && c.status == "True")}
      endpoint: ${cluster.status.endpoint}

  resources:
    - id: clusterRole
      template:
        apiVersion: iam.services.k8s.aws/v1alpha1
        kind: Role
        metadata:
          name: ${schema.metadata.name}
          annotations:
            services.k8s.aws/region: ${schema.spec.region}
        spec:
          name: ${schema.metadata.name}
          assumeRolePolicyDocument: |
            {"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"eks.amazonaws.com"},"Action":"sts:AssumeRole"}]}
          policies: ${schema.spec.clusterRolePolicies}
      readyWhen:
        - ${clusterRole.status.ackResourceMetadata.arn != ""}
        - ${clusterRole.status.conditions.exists(c, c.type == "ACK.ResourceSynced" && c.status == "True")}

    - id: cluster
      template:
        apiVersion: eks.services.k8s.aws/v1alpha1
        kind: Cluster
        metadata:
          name: ${schema.metadata.name}
          annotations:
            services.k8s.aws/region: ${schema.spec.region}
        spec:
          name: ${schema.metadata.name}
          version: ${schema.spec.version}
          roleARN: ${clusterRole.status.ackResourceMetadata.arn}
          resourcesVPCConfig:
            subnetIDs: ${schema.spec.vpc.subnetIDs}
            securityGroupIDs: ${schema.spec.vpc.securityGroupIDs}
            endpointPrivateAccess: ${schema.spec.vpc.endpointPrivateAccess}
            endpointPublicAccess: ${schema.spec.vpc.endpointPublicAccess}
            publicAccessCIDRs: ${schema.spec.vpc.publicAccessCIDRs}
      readyWhen:
        - ${cluster.status.status == "ACTIVE"}
        - ${cluster.status.endpoint != ""}
        - ${cluster.status.conditions.exists(c, c.type == "ACK.ResourceSynced" && c.status == "True")}
```

`readyWhen` は resource 自身（ここでは `cluster`）しか参照できないため、依存順序の制御は「他 resource の参照による DAG」+「resource 自身の readyWhen」の組み合わせで作ります。

### 8.4 親 RGD での status 統一

- 親 RGD（例: `PlatformCluster`）は provider-specific な部品RGD（例: `EKSControlPlane`）を chaining で呼び出します。
- Kany8s が参照するのは常に「親 RGD instance」とし、`status.ready` / `status.endpoint` を親の出力として統一します。

例（抜粋）:

```yaml
schema:
  status:
    ready: ${controlPlane.status.ready}
    endpoint: ${controlPlane.status.endpoint}
resources:
  - id: controlPlane
    template:
      apiVersion: kro.run/v1alpha1
      kind: EKSControlPlane
      # ...
    readyWhen:
      - ${controlPlane.status.ready == true}
```

### 8.5 Kany8s コントローラ側を最小化するための約束

- Kany8s コントローラは `kro instance.status.endpoint` を URL parse し、`Kany8sControlPlane.spec.controlPlaneEndpoint` と `Kany8sControlPlane.status.initialization.controlPlaneInitialized` を更新するだけに留める
- provider-specific な `kro instance.status.ready` / `kro instance.status.endpoint` の判定は RGD（および chaining した部品RGD）側に閉じ込める

## 9. 追加要件（CAPI contract / 将来拡張）

- kubeconfig Secret（CAPI contract 上 **必須**）
  - `<cluster>-kubeconfig` / type `cluster.x-k8s.io/secret` / label `cluster.x-k8s.io/cluster-name=${CLUSTER_NAME}`
  - `data.value` に kubeconfig を格納

- Provider 追加（AKS/GKE 等）
  - RGD 側で status 正規化を実装し、Kany8s コントローラは変更しない

- Infrastructure provider の扱い
  - CAPI の `Cluster.spec.infrastructureRef` をどう満たすか（Kany8s が infra も提供するか、既存 provider と併用するか）

---

## 参考（CAPT の該当箇所）

- endpoint 抽出（Workspace outputs/secret → host/port）: `capt/internal/controller/controlplane/endpoint/endpoint.go`
- endpoint を Cluster/ControlPlane に反映: `capt/internal/controller/controlplane/status.go`
- WorkspaceTemplate/Apply の API: `capt/api/v1beta2/workspacetemplate_types.go`, `capt/api/v1beta2/workspacetemplateapply_types.go`
