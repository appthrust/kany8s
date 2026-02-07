apiVersion: ec2.services.k8s.aws/v1alpha1
kind: VPC
metadata:
  name: __NETWORK_NAME__-vpc
  namespace: __NAMESPACE__
  annotations:
    services.k8s.aws/region: __AWS_REGION__
spec:
  cidrBlocks:
    - __VPC_CIDR__
  enableDNSSupport: true
  enableDNSHostnames: true
  tags:
    - key: Name
      value: __NETWORK_NAME__-vpc
---
apiVersion: ec2.services.k8s.aws/v1alpha1
kind: Subnet
metadata:
  name: __NETWORK_NAME__-subnet-a
  namespace: __NAMESPACE__
  annotations:
    services.k8s.aws/region: __AWS_REGION__
spec:
  vpcRef:
    from:
      name: __NETWORK_NAME__-vpc
  cidrBlock: __SUBNET_A_CIDR__
  availabilityZone: __SUBNET_A_AZ__
  mapPublicIPOnLaunch: true
  tags:
    - key: Name
      value: __NETWORK_NAME__-subnet-a
---
apiVersion: ec2.services.k8s.aws/v1alpha1
kind: Subnet
metadata:
  name: __NETWORK_NAME__-subnet-b
  namespace: __NAMESPACE__
  annotations:
    services.k8s.aws/region: __AWS_REGION__
spec:
  vpcRef:
    from:
      name: __NETWORK_NAME__-vpc
  cidrBlock: __SUBNET_B_CIDR__
  availabilityZone: __SUBNET_B_AZ__
  mapPublicIPOnLaunch: true
  tags:
    - key: Name
      value: __NETWORK_NAME__-subnet-b
