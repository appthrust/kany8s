# examples/simple

Minimal end-user manifests for creating managed control planes with Kany8s.

## EKS (ACK + kro)

1. Install kro + Kany8s (see `README.md`)
2. Install ACK controllers (IAM + EKS): `docs/runbooks/ack.md`
3. Apply the EKS control plane RGD:

   `kubectl apply -f examples/simple/eks-control-plane-rgd.yaml`

4. Apply the cluster:

   `kubectl apply -f examples/simple/eks-cluster.yaml`

Notes:

- This sample creates the managed control plane only (no worker nodes).
- If Cluster API core isn't installed, apply only the `Kany8sControlPlane` from `examples/simple/eks-cluster.yaml`.
- Replace `subnet-xxxx` / `sg-zzzz` with real IDs.
