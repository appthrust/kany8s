# Acceptance Test Runners

This directory contains thin wrappers around the existing acceptance scripts in `hack/`.

They focus on reproducibility:

- delete the target kind cluster before running
- (self-managed) delete leftover workload Docker containers before running
- delegate the real work and cleanup to the `hack/` scripts

## Manifests

Project-owned YAML that the acceptance scripts apply lives under:

- `test/acceptance_test/manifests/`

Third-party YAML is downloaded on-demand (and cached) under:

- `test/acceptance_test/vendor/`

## Scripts

- `test/acceptance_test/run-acceptance-kro-reflection.sh`
  - wraps `hack/acceptance-test-kro-reflection.sh`
  - Purpose: validate managed-kro "status reflection" (kro instance -> `Kany8sControlPlane` endpoint/initialized/Ready)
- `test/acceptance_test/run-acceptance-kro-reflection-multi-rgd.sh`
  - wraps `hack/acceptance-test-kro-reflection-multi-rgd.sh` (one CAPI kind, two different RGDs / instance kinds)
  - Purpose: prove `Kany8sControlPlane` can drive multiple kro instance kinds via `spec.resourceGraphDefinitionRef`
- `test/acceptance_test/run-acceptance-kro-infra-reflection.sh`
  - wraps `hack/acceptance-test-kro-infra-reflection.sh`
- `test/acceptance_test/run-acceptance-capd-kubeadm.sh`
  - wraps `hack/acceptance-test-capd-kubeadm.sh`
  - Purpose: validate self-managed provisioning (CAPD + kubeadm) creates a reachable workload cluster

Legacy aliases are still supported:

- `test/acceptance_test/run-acceptance.sh`
- `test/acceptance_test/run-acceptance-multi-rgd.sh`
- `test/acceptance_test/run-acceptance-self-managed.sh`
- `test/acceptance_test/run-e2e.sh`
  - wraps `make test-e2e` (with pre-clean + always-cleanup)
  - Purpose: smoke e2e (deploy controller to kind; verify it runs + serves metrics)
- `test/acceptance_test/run-all.sh`
  - runs acceptance flows sequentially (kro reflection + multi-rgd + self-managed)

## Typical usage

```bash
# kro-based acceptance (kind only)
test/acceptance_test/run-acceptance-kro-reflection.sh

# self-managed (kind + clusterctl + CAPD + kubeadm)
test/acceptance_test/run-acceptance-capd-kubeadm.sh

# e2e (kind + deploy + metrics)
test/acceptance_test/run-e2e.sh

# run both
test/acceptance_test/run-all.sh
```

## Common environment variables

- `CLEANUP` (default: `true`)
- `ARTIFACTS_DIR` (default: under `/tmp/...`)
- `KIND_CLUSTER_NAME` (default: generated from timestamp)

See each `hack/acceptance-test*.sh` for the full list.
