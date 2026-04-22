# eks-karpenter-bootstrapper

Kany8s EKS Karpenter bootstrapper — a controller that provisions the AWS
side-cars Karpenter needs (IAM Role, OIDC provider, SecurityGroup, Fargate
profile) and installs Karpenter via Flux on CAPI-managed EKS clusters.

## TL;DR

```bash
helm install bootstrapper oci://ghcr.io/appthrust/charts/eks-karpenter-bootstrapper \
  --version 0.1.1 \
  --namespace kany8s-eks-system --create-namespace \
  --set aws.mode=irsa \
  --set aws.irsa.roleArn=arn:aws:iam::123456789012:role/eks-karpenter-bootstrapper
```

## Prerequisites

- Kubernetes `>=1.27` with the kany8s manager running.
- ACK EKS / IAM / EC2 controllers installed (the bootstrapper reconciles
  `eks.services.k8s.aws/*`, `iam.services.k8s.aws/*`, `ec2.services.k8s.aws/*`).
- Flux installed (`source.toolkit.fluxcd.io` + `helm.toolkit.fluxcd.io`). The
  bootstrapper emits `OCIRepository` + `HelmRelease` for upstream Karpenter.
- One of the AWS credential modes below.

## AWS credential modes

| Mode | When to use | What the chart wires |
|---|---|---|
| `staticSecret` | Local dev, kind clusters, quick POC. | Mounts a `Secret` at `/aws/credentials` and sets `AWS_SHARED_CREDENTIALS_FILE`. |
| `irsa` | Production on EKS with an OIDC provider. | Annotates the `ServiceAccount` with `eks.amazonaws.com/role-arn`. |
| `podIdentity` | EKS 2023+ with Pod Identity Agent installed. | No in-pod config; AWS SDK uses `AWS_CONTAINER_CREDENTIALS_FULL_URI` injected by the agent. |

Exactly one mode is active per release (switch via `aws.mode`).

## Image override recipes

```bash
# 1. tag-only override (default registry / repository remain)
helm install bootstrapper oci://ghcr.io/appthrust/charts/eks-karpenter-bootstrapper \
  --version 0.1.1 \
  --set image.tag=v0.1.1

# 2. private mirror (keep repository path, swap registry)
helm install bootstrapper oci://ghcr.io/appthrust/charts/eks-karpenter-bootstrapper \
  --version 0.1.1 \
  --set image.registry=my-registry.example.com \
  --set imagePullSecrets[0].name=my-registry-creds

# 3. air-gap / rebranded artifact (independent registry + repository + tag)
helm install bootstrapper oci://ghcr.io/appthrust/charts/eks-karpenter-bootstrapper \
  --version 0.1.1 \
  --set image.registry=registry.internal \
  --set image.repository=platform/kany8s-eks-bootstrapper \
  --set image.tag=2026.04.22-internal

# 4. immutable by digest (tag is ignored when digest is set)
helm install bootstrapper oci://ghcr.io/appthrust/charts/eks-karpenter-bootstrapper \
  --version 0.1.1 \
  --set image.digest=sha256:<64 hex chars>

# 5. override registry across every chart in a release (global.imageRegistry)
helm install bootstrapper oci://ghcr.io/appthrust/charts/eks-karpenter-bootstrapper \
  --version 0.1.1 \
  --set global.imageRegistry=my-registry.example.com
```

## Values

See [`values.yaml`](values.yaml) for the full list; highlights:

| Key | Default | Notes |
|---|---|---|
| `namespace` | `kany8s-eks-system` | Target namespace (the chart can create it). |
| `createNamespace` | `true` | Skip if another tool owns the namespace. |
| `image.registry` / `repository` / `tag` / `digest` | `ghcr.io` / `appthrust/kany8s/eks-karpenter-bootstrapper` / `""` / `""` | Tag falls back to `Chart.appVersion` when empty. `digest` wins over `tag`. |
| `global.imageRegistry` | `""` | When set, overrides `image.registry`. |
| `aws.mode` | `staticSecret` | `staticSecret` / `irsa` / `podIdentity`. |
| `aws.irsa.roleArn` | `""` | Required when `aws.mode=irsa`. |
| `args.failureBackoff` | `30s` | Requeue interval when prerequisites are not ready. |
| `args.steadyStateRequeue` | `10m` | Requeue interval after a successful reconciliation. |
| `args.karpenterChartVersion` | `""` | Override Flux `OCIRepository.spec.ref.tag` for the upstream Karpenter chart. Empty uses the controller default. |

## Relationship to `clusterctl` / cluster-api-operator

`clusterctl` and cluster-api-operator install only the provider managers
(Infrastructure / ControlPlane). The EKS-specific plugins are outside that
contract and must be installed separately with Helm. See the root
[`README.md`](https://github.com/appthrust/kany8s#installing-eks-plugins-via-helm)
for the quickstart.

## Source

- Controller source: `cmd/eks-karpenter-bootstrapper/main.go`, `internal/controller/plugin/eks/karpenter_bootstrapper_controller.go`
- Kustomize overlay (legacy / dev): `config/eks-karpenter-bootstrapper/`
- Companion chart: `eks-kubeconfig-rotator`
