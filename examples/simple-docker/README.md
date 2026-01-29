# examples/simple-docker

Minimal end-user manifests for creating a Cluster API `Cluster` with:

- CAPD (`DockerCluster`) as the InfrastructureRef
- Kany8s (`Kany8sControlPlane`) as the ControlPlaneRef (via kro RGD)

## Docker (CAPD + kro)

1. Install kro + Kany8s (see repo `README.md`)
2. Install Cluster API providers (CAPD): `docs/runbooks/clusterctl.md`
3. Apply the demo control plane RGD:

   `kubectl apply -f examples/simple-docker/docker-control-plane-rgd.yaml`

4. Apply the cluster:

   `kubectl apply -f examples/simple-docker/docker-cluster.yaml`

Notes:

- This sample is intentionally minimal (no Machines/worker nodes).
- The RGD deploys an `nginx` Service as a fake "control plane endpoint"; it is not a real Kubernetes API server.
- If Cluster API/CAPD isn't installed, apply only the `Kany8sControlPlane` from `examples/simple-docker/docker-cluster.yaml`.
