# eks-kubeconfig-rotator

Kany8s EKS kubeconfig rotator — a controller that watches CAPI `Cluster` and
`eks.services.k8s.aws/Cluster` pairs and keeps the CAPI kubeconfig `Secret`
fresh by rotating short-lived EKS tokens before they expire.

Without this controller the kubeconfig `Secret` minted during EKS bootstrap
expires after ~15 minutes and the CAPI `Cluster` never reaches
`Available=True`.

## TL;DR

```bash
helm install rotator oci://ghcr.io/appthrust/charts/eks-kubeconfig-rotator \
  --version 0.1.1 \
  --namespace kany8s-eks-system --create-namespace \
  --set aws.mode=irsa \
  --set aws.irsa.roleArn=arn:aws:iam::123456789012:role/eks-rotator
```

## Prerequisites

- Kubernetes `>=1.27` with the kany8s manager running (CAPI `Cluster` CRDs plus
  the `eks.services.k8s.aws/Cluster` CRD from ACK EKS controller).
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
helm install rotator oci://ghcr.io/appthrust/charts/eks-kubeconfig-rotator \
  --version 0.1.1 \
  --set image.tag=v0.1.1

# 2. private mirror (keep repository path, swap registry)
helm install rotator oci://ghcr.io/appthrust/charts/eks-kubeconfig-rotator \
  --version 0.1.1 \
  --set image.registry=my-registry.example.com \
  --set imagePullSecrets[0].name=my-registry-creds

# 3. air-gap / rebranded artifact (independent registry + repository + tag)
helm install rotator oci://ghcr.io/appthrust/charts/eks-kubeconfig-rotator \
  --version 0.1.1 \
  --set image.registry=registry.internal \
  --set image.repository=platform/kany8s-eks-rotator \
  --set image.tag=2026.04.22-internal

# 4. immutable by digest (tag is ignored when digest is set)
helm install rotator oci://ghcr.io/appthrust/charts/eks-kubeconfig-rotator \
  --version 0.1.1 \
  --set image.digest=sha256:<64 hex chars>

# 5. override registry across every chart in a release (global.imageRegistry)
helm install rotator oci://ghcr.io/appthrust/charts/eks-kubeconfig-rotator \
  --version 0.1.1 \
  --set global.imageRegistry=my-registry.example.com
```

## Values

See [`values.yaml`](values.yaml) for the full list; highlights:

| Key | Default | Notes |
|---|---|---|
| `namespace` | `kany8s-eks-system` | Target namespace (the chart can create it). |
| `createNamespace` | `true` | Skip if another tool owns the namespace. |
| `image.registry` / `repository` / `tag` / `digest` | `ghcr.io` / `appthrust/kany8s/eks-kubeconfig-rotator` / `""` / `""` | Tag falls back to `Chart.appVersion` when empty. `digest` wins over `tag`. |
| `global.imageRegistry` | `""` | When set, overrides `image.registry`. |
| `aws.mode` | `staticSecret` | `staticSecret` / `irsa` / `podIdentity`. |
| `aws.irsa.roleArn` | `""` | Required when `aws.mode=irsa`. |
| `args.refreshBefore` | `5m` | Start rotating when the token has less than this remaining. |
| `args.maxRefreshInterval` | `10m` | Upper bound on the requeue interval while the token is still valid. |
| `args.failureBackoff` | `30s` | Requeue interval when prerequisites are not ready. |

## Relationship to `clusterctl` / cluster-api-operator

`clusterctl` and cluster-api-operator install only the provider managers
(Infrastructure / ControlPlane). The EKS-specific plugins are outside that
contract and must be installed separately with Helm. See the root
[`README.md`](https://github.com/appthrust/kany8s#installing-eks-plugins-via-helm)
for the quickstart.

## Source

- Controller source: `cmd/eks-kubeconfig-rotator/main.go`, `internal/controller/plugin/eks/rotator_controller.go`
- Kustomize overlay (legacy / dev): `config/eks-plugin/`
- Companion chart: `eks-karpenter-bootstrapper`
