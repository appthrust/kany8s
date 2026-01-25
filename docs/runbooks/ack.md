# ACK (AWS Controllers for Kubernetes)

This runbook installs the ACK controllers needed by the `examples/kro/eks/` RGDs.

## Prerequisites

- An AWS account (use a sandbox account for testing)
- AWS credentials that can create EKS clusters and IAM roles
- `kubectl`
- `helm`

## 1. Export AWS environment variables

```bash
export AWS_REGION=ap-northeast-1

export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...

# If you use temporary credentials (STS), also export:
export AWS_SESSION_TOKEN=...
```

## 2. Create the ACK credentials secret

ACK Helm charts can source credentials from a Kubernetes Secret.

```bash
kubectl create namespace ack-system

kubectl delete secret ack-user-secrets -n ack-system --ignore-not-found
kubectl create secret generic ack-user-secrets -n ack-system \
  --from-literal=aws_access_key_id="${AWS_ACCESS_KEY_ID}" \
  --from-literal=aws_secret_access_key="${AWS_SECRET_ACCESS_KEY}" \
  --from-literal=aws_session_token="${AWS_SESSION_TOKEN}"
```

If you are not using STS credentials, omit `aws_session_token`.

## 3. Install ACK controllers (IAM + EKS)

This project currently uses ACK IAM + ACK EKS resources:

- IAM: `iam.services.k8s.aws/v1alpha1` `Role`
- EKS: `eks.services.k8s.aws/v1alpha1` `Cluster` / `Addon` / `PodIdentityAssociation`

Install both controllers into the same namespace.

These commands use `helm upgrade --install` (same as `helm install` on first run).

```bash
helm upgrade --install ack-iam-controller \
  oci://public.ecr.aws/aws-controllers-k8s/iam-chart \
  --namespace ack-system \
  --set aws.region="${AWS_REGION}" \
  --set aws.credentials.secretName=ack-user-secrets

helm upgrade --install ack-eks-controller \
  oci://public.ecr.aws/aws-controllers-k8s/eks-chart \
  --namespace ack-system \
  --set aws.region="${AWS_REGION}" \
  --set aws.credentials.secretName=ack-user-secrets
```

Note: you can pin controller versions by adding `--version <tag>` to each `helm` command.

## 4. Verify installation

```bash
kubectl get deploy -n ack-system

kubectl get crd | grep -E '(clusters|addons|podidentityassociations)\.eks\.services\.k8s\.aws|roles\.iam\.services\.k8s\.aws'
```

## 5. Cleanup

```bash
helm uninstall ack-eks-controller -n ack-system
helm uninstall ack-iam-controller -n ack-system

kubectl delete namespace ack-system
```
