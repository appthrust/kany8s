# eks-karpenter-bootstrapper

Kany8s EKS Karpenter bootstrapper — a controller that provisions the AWS
side-cars Karpenter needs (IAM Role, OIDC provider, SecurityGroup, Fargate
profile) and installs Karpenter via Flux on CAPI-managed EKS clusters.

## TL;DR

```bash
helm install bootstrapper oci://ghcr.io/appthrust/charts/eks-karpenter-bootstrapper \
  --version 0.1.1 \
  --namespace kany8s-eks-system --create-namespace \
  --set "serviceAccount.annotations.eks\.amazonaws\.com/role-arn=arn:aws:iam::123456789012:role/eks-karpenter-bootstrapper"
```

## Prerequisites

- Kubernetes `>=1.27` with the kany8s manager running.
- ACK EKS / IAM / EC2 controllers installed (the bootstrapper reconciles
  `eks.services.k8s.aws/*`, `iam.services.k8s.aws/*`, `ec2.services.k8s.aws/*`).
- Flux installed (`source.toolkit.fluxcd.io` + `helm.toolkit.fluxcd.io`). The
  bootstrapper emits `OCIRepository` + `HelmRelease` for upstream Karpenter.
- One of the AWS credential sources below.

## AWS credentials

The chart follows the ACK controller convention: if `aws.credentials.secretName`
is non-empty, a shared credentials file Secret is mounted; otherwise the AWS
SDK default credential chain applies (IRSA, EKS Pod Identity, EC2 IMDS).

| Source | What to configure | How the chart wires it |
|---|---|---|
| Shared credentials Secret (local dev, kind) | `aws.credentials.secretName=<secret>` (optionally `aws.credentials.secretKey` / `aws.credentials.profile`) | Mounts the Secret at `/var/run/secrets/aws/` and sets `AWS_SHARED_CREDENTIALS_FILE` / `AWS_PROFILE`. |
| IRSA (production EKS + OIDC) | `serviceAccount.annotations.eks\.amazonaws\.com/role-arn=<role arn>` | Stamps the annotation on the ServiceAccount; SDK picks the role up via the OIDC token projection. |
| EKS Pod Identity (EKS 2023+) | Create a `PodIdentityAssociation` for the chart's ServiceAccount (out of band). | No in-pod config; SDK uses `AWS_CONTAINER_CREDENTIALS_FULL_URI` injected by the agent. |
| EC2 instance profile / self-managed nodes | Attach an IAM role to the underlying node. | No in-pod config; SDK falls back to IMDS. |

Only one source is active at a time — `aws.credentials.secretName` wins over
whatever the SDK default chain would resolve next.

The Secret format is identical to the one ACK controllers consume, so the
same Secret (e.g. `aws-creds` created during ACK setup) can be reused by
setting `aws.credentials.secretName=aws-creds`.

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
| `aws.credentials.secretName` | `""` | Non-empty switches the chart into shared-credentials-file mode (ACK format). Empty falls back to the SDK default chain. |
| `aws.credentials.secretKey` | `credentials` | Key inside the Secret holding the INI file body. |
| `aws.credentials.profile` | `default` | Profile selected via `AWS_PROFILE`. |
| `serviceAccount.annotations` | `{}` | Set `eks.amazonaws.com/role-arn` here for IRSA. |
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
