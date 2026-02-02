# RGD Guidelines

This document captures practical guidelines for authoring kro `ResourceGraphDefinition` (RGD) objects that work well with Kany8s.

For deeper investigation notes and minimal reproductions on kind + kro v0.7.1, see `docs/kro.md`.

## Static Analysis / Validation

kro performs static analysis when an RGD is applied. Before publishing an RGD:

1. Apply the RGD to a test cluster where all referenced API types exist (install any required CRDs/controllers first).
2. Confirm the RGD is accepted:
   - `kubectl get resourcegraphdefinitions.kro.run <name> -o wide`
   - `kubectl describe resourcegraphdefinition <name>`
3. Ensure `ResourceGraphAccepted=True` and fix any reported schema / template errors.

If you reference external CRDs (ACK / Config Connector / ASO, etc.), note that missing CRDs can cause the static analysis to fail even if the YAML is otherwise correct.

## Known kro v0.7.1 Pitfalls (Do/Don't)

### `spec.schema.status` CEL environment

- Don't reference `schema.*` inside `spec.schema.status` (kro rejects it).
- Do compute status fields from resource id variables (for example `${cluster.status.endpoint}`).

### `readyWhen` scope

- `readyWhen` may only reference the self resource.
- Don't reference other resources or `schema.*` from `readyWhen`.

### Status string formatting

- Avoid the YAML "string template" form (e.g. `"http://${service.metadata.name}"`), which can drop literals.
- Prefer a single CEL expression (e.g. `${"http://" + service.metadata.name}`).

### Status fields must refer to resources

- kro rejects status fields that are pure constants.
- Ensure each status field expression refers to at least one resource id variable.

### Optional resources and missing fields

- If an optional resource (`includeWhen=false`) is referenced from status, the status field can be omitted entirely.
- If a status field must always exist (like `status.ready` / `status.endpoint`), avoid depending on optional resources.

## Infra outputs into control plane spec (Approach A)

If a parent RGD composes infrastructure resources and a managed control plane resource, it is common to wire infra outputs (VPC IDs, subnet IDs, security group IDs, etc.) into the control plane spec.

During the "infra not ready yet" phase, referenced fields can be missing. If you reference missing fields directly, kro can fail template evaluation and the parent graph can get stuck.

Guidelines:

- Use optional field selection (`.?`) anywhere a field might be missing.
- Provide safe defaults with `orValue(...)` so template evaluation succeeds while waiting.
- Guard each nesting level that might be missing (for example `status.?ackResourceMetadata.?arn`).

Examples (CEL snippets embedded in YAML templates):

```yaml
spec:
  # Default strings to empty string
  roleARN: ${role.status.?ackResourceMetadata.?arn.orValue("")}

  # Default arrays to empty list
  subnetIDs: ${vpc.status.?subnetIDs.orValue([])}

  # Default objects to empty object (when the target field expects an object)
  tags: ${someResource.status.?tags.orValue({})}
```

If the downstream CRD has strict validation (for example, a required field that must be non-empty or match a pattern), do NOT default to an invalid placeholder. Instead, gate the dependent resource using `includeWhen` so it is only created once inputs exist:

```yaml
resources:
  - id: controlPlane
    includeWhen: ${vpc.status.?vpcID.orValue("") != ""}
    template:
      apiVersion: example.kro.run/v1alpha1
      kind: ExampleControlPlane
      spec:
        vpcID: ${vpc.status.?vpcID.orValue("")}
```

### Boolean materialization quirks

- Some boolean status expressions may not materialize as fields.
- Workarounds include casting a numeric operand (e.g. `int(<number>) == 1`) so the boolean field is reliably present.
- Note: kro v0.7.1 rejects `int(<bool>)`; use a ternary to convert booleans if needed (e.g. `int((<bool>) ? 1 : 0) == 1`).

### `NetworkPolicy` can block readiness

- In kind testing with kro v0.7.1, graphs containing `NetworkPolicy` can get stuck `IN_PROGRESS` and never become Ready.
- Avoid including `NetworkPolicy` in RGD graphs for now; apply it separately or track upstream fixes.

## Kany8s Compatibility

If an RGD is intended to back `Kany8sControlPlane`, it MUST follow the normalized status contract in `docs/rgd-contract.md` (`status.ready`, `status.endpoint`, etc.).
