# Runbook: ACK (AWS Controllers for Kubernetes)

This runbook installs ACK service controllers that are commonly used by an EKS-focused kro RGD
(for example: IAM + EKS).

## Repro environment

- Helm: 3.8+
- ACK controllers: installed via Helm from `public.ecr.aws/aws-controllers-k8s/*-chart`

## Prereqs

- A Kubernetes cluster (management cluster)
- AWS account + credentials
- awscli (for ECR public registry login)
- helm
- (recommended) jq (to pick latest controller release tag)

## Install (Helm)

1) Login to the ECR public Helm registry:

```bash
aws ecr-public get-login-password --region us-east-1 \
  | helm registry login --username AWS --password-stdin public.ecr.aws
```

2) Install the controllers you need (example: IAM + EKS):

```bash
export ACK_SYSTEM_NAMESPACE=ack-system
export AWS_REGION=ap-northeast-1

for SERVICE in iam eks; do
  RELEASE_VERSION=$(curl -sL https://api.github.com/repos/aws-controllers-k8s/${SERVICE}-controller/releases/latest \
    | jq -r '.tag_name | ltrimstr("v")')

  helm install --create-namespace -n ${ACK_SYSTEM_NAMESPACE} ack-${SERVICE}-controller \
    oci://public.ecr.aws/aws-controllers-k8s/${SERVICE}-chart \
    --version=${RELEASE_VERSION} \
    --set=aws.region=${AWS_REGION}
done

kubectl -n ${ACK_SYSTEM_NAMESPACE} rollout status deploy/ack-iam-controller
kubectl -n ${ACK_SYSTEM_NAMESPACE} rollout status deploy/ack-eks-controller
```

## Credentials (short version)

ACK uses the default AWS SDK credential chain inside the controller pods.

- Recommended on EKS: IRSA / web identity token file.
- On kind / non-EKS clusters: mount a shared credentials file or set env vars on the Deployment.

Example (env vars; not recommended for production):

```bash
kubectl -n ${ACK_SYSTEM_NAMESPACE} set env deploy/ack-iam-controller \
  AWS_ACCESS_KEY_ID="$AWS_ACCESS_KEY_ID" \
  AWS_SECRET_ACCESS_KEY="$AWS_SECRET_ACCESS_KEY" \
  AWS_SESSION_TOKEN="$AWS_SESSION_TOKEN"

kubectl -n ${ACK_SYSTEM_NAMESPACE} set env deploy/ack-eks-controller \
  AWS_ACCESS_KEY_ID="$AWS_ACCESS_KEY_ID" \
  AWS_SECRET_ACCESS_KEY="$AWS_SECRET_ACCESS_KEY" \
  AWS_SESSION_TOKEN="$AWS_SESSION_TOKEN"
```

## Observe

```bash
kubectl -n ${ACK_SYSTEM_NAMESPACE} get deploy,pods
kubectl get crd | grep services.k8s.aws

# IAM
kubectl get roles.iam.services.k8s.aws -A -o wide
kubectl describe roles.iam.services.k8s.aws -n <ns> <name>

# EKS
kubectl get clusters.eks.services.k8s.aws -A -o wide
kubectl describe clusters.eks.services.k8s.aws -n <ns> <name>

# ACK conditions and status fields
kubectl get clusters.eks.services.k8s.aws -n <ns> <name> -o jsonpath='{.status.conditions}' && echo
```

References:

- Install: https://aws-controllers-k8s.github.io/community/docs/user-docs/install/
- Credentials: https://aws-controllers-k8s.github.io/community/docs/user-docs/authentication/
