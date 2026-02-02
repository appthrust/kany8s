# TODO

This file tracks *current* work items. Historical checklists were removed to keep this repo navigable; use `git log` for completed work.

## Current Focus: Kany8sCluster infra provider (kro integration)

- [ ] Define infra RGD status contract (Infrastructure): `status.ready/reason/message`
  - Touch: `docs/rgd-contract.md`
  - DoD: infra RGD authors know exactly what fields to emit

- [ ] Extend `Kany8sCluster` API to select an RGD
  - Add: `spec.resourceGraphDefinitionRef.name` (+ any minimal injection rules)
  - Touch: `api/infrastructure/v1alpha1/kany8scluster_types.go`
  - DoD: `make manifests generate test` passes

- [ ] Align `Kany8sClusterTemplate` with the new `Kany8sCluster` inputs
  - Touch: `api/infrastructure/v1alpha1/kany8sclustertemplate_types.go`
  - DoD: ClusterClass/Topology can generate a `Kany8sCluster` with the needed fields

- [ ] Implement `Kany8sCluster` controller kro integration (infrastructure concretization)
  - Resolve instance GVK from RGD, create/update kro instance (unstructured)
  - Drive `status.initialization.provisioned` from instance `status.ready`
  - Touch: `internal/controller/infrastructure/kany8scluster_controller.go`
  - DoD: unit tests cover create/update + provisioned transitions

- [ ] Add examples for infra RGD + wiring
  - Add: `examples/kro/<infra>/...`
  - Update: `examples/capi/cluster.yaml` (or add a new example)
  - DoD: `kubectl apply` demonstrates `Kany8sCluster` becoming Provisioned via RGD instance

## Decisions Needed

- [ ] Decide how infra outputs (e.g., VPC IDs) feed into control plane without introducing “generic outputs” as a core concept
  - Candidate MVPs: naming conventions, or a parent RGD that owns both infra + control plane
