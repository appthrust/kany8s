# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Kany8s is a Cluster API provider suite (`Kany8sCluster` + `Kany8sControlPlane`) that uses **kro** (ResourceGraphDefinition/RGD) as a concretization engine to create managed Kubernetes control planes on any cloud provider. The controller is provider-agnostic—it reads only the kro instance status, not cloud-specific CRs.

## Build & Test Commands

```bash
make test              # Unit tests (includes manifests + generate)
make lint              # golangci-lint check
make lint-fix          # golangci-lint with auto-fixes
make run               # Run controller locally against kubeconfig
make manifests         # Generate CRDs & RBAC from markers
make generate          # Generate DeepCopy methods
make install           # Install CRDs to cluster
make deploy IMG=<img>  # Deploy controller to cluster
```

### E2E and Acceptance Tests

```bash
make test-e2e              # E2E tests (creates isolated Kind cluster)

make test-acceptance-kro-reflection       # kro status -> Kany8sControlPlane reflection
make test-acceptance-kro-reflection-keep  # Keep cluster after kro reflection test
make test-acceptance-kro-reflection-multi-rgd  # Same, but proves multiple RGDs / instance kinds

make test-acceptance-capd-kubeadm          # CAPD + kubeadm (real workload cluster)
make test-acceptance-capd-kubeadm-keep     # Keep cluster after CAPD+kubeadm test

# Legacy aliases still exist:
# - make test-acceptance -> make test-acceptance-kro-reflection
# - make test-acceptance-self-managed -> make test-acceptance-capd-kubeadm
# - make test-acceptance-multi-rgd -> make test-acceptance-kro-reflection-multi-rgd
```

## Architecture

### Multi-Group Kubebuilder Layout

This project uses a multi-group layout (`PROJECT: multigroup: true`):

- `api/v1alpha1/` - ControlPlane CRDs (`Kany8sControlPlane`, `Kany8sControlPlaneTemplate`)
- `api/infrastructure/v1alpha1/` - Infrastructure CRDs (`Kany8sCluster`, `Kany8sClusterTemplate`)
- `internal/controller/` - Reconciliation logic
- `internal/controller/infrastructure/` - Infrastructure controllers

### Key Internal Packages

| Package | Purpose |
|---------|---------|
| `internal/dynamicwatch/` | Watches kro instances with dynamic GVK (unknown at startup) |
| `internal/endpoint/` | Parses/validates API endpoints |
| `internal/kro/` | RGD resolution, instance GVK lookup |
| `internal/kubeconfig/` | Kubeconfig Secret management |
| `internal/devtools/` | Test suite for design validation |

### Controller Flow

1. `Kany8sControlPlane` references a kro `ResourceGraphDefinition` via `spec.resourceGraphDefinitionRef`
2. Controller resolves RGD's generated GVK and creates a kro instance (1:1)
3. Controller watches **only** the kro instance `status`
4. When kro instance reports `ready` + `endpoint`, controller sets:
   - `Kany8sControlPlane.spec.controlPlaneEndpoint`
   - `status.initialization.controlPlaneInitialized`

### RGD Status Contract

Kany8s expects RGD instances to expose:
- `status.ready` (boolean, required)
- `status.endpoint` (string, required, format: `https://host[:port]`)
- `status.reason` (string, optional)
- `status.message` (string, optional)

See `docs/reference/rgd-contract.md` for full specification.

## Critical Rules

### Never Edit Auto-Generated Files
- `config/crd/bases/*.yaml` - from `make manifests`
- `config/rbac/role.yaml` - from `make manifests`
- `**/zz_generated.*.go` - from `make generate`
- `PROJECT` - from kubebuilder CLI

### Never Remove Scaffold Markers
Do NOT delete `// +kubebuilder:scaffold:*` comments. CLI injects code at these markers.

### After API Changes
```bash
make manifests generate test
```

### Always Use CLI for New Resources
```bash
kubebuilder create api --group <group> --version <version> --kind <Kind>
kubebuilder create webhook --group <group> --version <version> --kind <Kind> --defaulting --programmatic-validation
```

## Testing

- Framework: **Ginkgo v2 + Gomega** (BDD-style)
- Unit tests use **envtest** (real K8s API + etcd, no cluster)
- E2E tests require an isolated **Kind** cluster
- Test files: Located alongside code with `_test.go` suffix

## Key Documentation

- `AGENTS.md` - Comprehensive Kubebuilder development guide
- `docs/adr/README.md` - Architecture decisions (ADR)
- `docs/reference/rgd-contract.md` - RGD status contract specification
- `docs/reference/rgd-guidelines.md` - RGD authoring best practices
- `docs/reference/kro-v0.7.1-kind-notes.md` - kro pitfalls (kind + kro v0.7.1)
- `docs/guides/e2e-and-acceptance-test.md` - E2E/Acceptance layers and runners
- `TODO.md` - Prioritized task list
