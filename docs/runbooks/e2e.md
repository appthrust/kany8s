# E2E (Cluster -> endpoint/initialized) troubleshooting

This runbook is a quick command checklist for debugging the flow from applying a CAPI `Cluster` to seeing:

- `Kany8sControlPlane.spec.controlPlaneEndpoint` populated
- `Kany8sControlPlane.status.initialization.controlPlaneInitialized=true`

## 0. Set variables

```bash
export NAMESPACE=default
export CLUSTER_NAME=demo-cluster
export CONTROLPLANE_NAME="${CLUSTER_NAME}"
```

## 1. Watch the high-level resources

```bash
kubectl get clusters -A -w

kubectl get kany8scontrolplanes -A -w
```

If you prefer fully-qualified resource names:

```bash
kubectl get kany8scontrolplanes.controlplane.cluster.x-k8s.io -A -w
```

## 2. Describe for conditions and error messages

```bash
kubectl describe cluster -n "${NAMESPACE}" "${CLUSTER_NAME}"

kubectl describe kany8scontrolplane -n "${NAMESPACE}" "${CONTROLPLANE_NAME}"
```

## 3. Check Events

```bash
kubectl get events -A --sort-by=.metadata.creationTimestamp
```

## 4. Inspect the referenced RGD and kro instance

The `Kany8sControlPlane` references an RGD by name:

```bash
kubectl get kany8scontrolplane -n "${NAMESPACE}" "${CONTROLPLANE_NAME}" \
  -o jsonpath='{.spec.resourceGraphDefinitionRef.name}{"\n"}'
```

Then inspect the RGD schema (this tells you the generated instance GVK):

```bash
export RGD_NAME=...  # set from the command above

kubectl get resourcegraphdefinitions.kro.run "${RGD_NAME}" -o yaml

kubectl get resourcegraphdefinitions.kro.run "${RGD_NAME}" \
  -o jsonpath='{.spec.schema.apiVersion}{" "}{.spec.schema.kind}{"\n"}'
```

The kro instance is created 1:1 with the same name/namespace as the `Kany8sControlPlane`.
Once you know the instance Kind, you can inspect its normalized status:

```bash
kubectl get <kind> -n "${NAMESPACE}" "${CONTROLPLANE_NAME}" -o yaml
```

Focus on:

- `.status.ready`
- `.status.endpoint`
- `.status.reason` / `.status.message` (if present)

## 5. Tail controller logs

```bash
kubectl logs -n kany8s-system deploy/kany8s-controller-manager -c manager -f
```

If kro itself is stuck, also check the kro controller logs:

```bash
kubectl logs -n kro-system deploy/kro -f
```

## 6. (If using the ACK EKS example) inspect ACK resources

```bash
kubectl get clusters.eks.services.k8s.aws -A
kubectl describe cluster.eks.services.k8s.aws -n "${NAMESPACE}" "${CLUSTER_NAME}"

kubectl get roles.iam.services.k8s.aws -A
```
