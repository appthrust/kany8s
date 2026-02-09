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
kind: InternetGateway
metadata:
  name: __NETWORK_NAME__-igw
  namespace: __NAMESPACE__
  annotations:
    services.k8s.aws/region: __AWS_REGION__
spec:
  vpcRef:
    from:
      name: __NETWORK_NAME__-vpc
  tags:
    - key: Name
      value: __NETWORK_NAME__-igw
---
apiVersion: ec2.services.k8s.aws/v1alpha1
kind: ElasticIPAddress
metadata:
  name: __NETWORK_NAME__-eip-nat
  namespace: __NAMESPACE__
  annotations:
    services.k8s.aws/region: __AWS_REGION__
spec:
  tags:
    - key: Name
      value: __NETWORK_NAME__-eip-nat
---
apiVersion: ec2.services.k8s.aws/v1alpha1
kind: RouteTable
metadata:
  name: __NETWORK_NAME__-rtb-public
  namespace: __NAMESPACE__
  annotations:
    services.k8s.aws/region: __AWS_REGION__
spec:
  vpcRef:
    from:
      name: __NETWORK_NAME__-vpc
  routes:
    - destinationCIDRBlock: 0.0.0.0/0
      gatewayRef:
        from:
          name: __NETWORK_NAME__-igw
  tags:
    - key: Name
      value: __NETWORK_NAME__-rtb-public
---
apiVersion: ec2.services.k8s.aws/v1alpha1
kind: Subnet
metadata:
  name: __NETWORK_NAME__-subnet-public-a
  namespace: __NAMESPACE__
  annotations:
    services.k8s.aws/region: __AWS_REGION__
spec:
  vpcRef:
    from:
      name: __NETWORK_NAME__-vpc
  cidrBlock: __PUBLIC_SUBNET_A_CIDR__
  availabilityZone: __PUBLIC_SUBNET_A_AZ__
  mapPublicIPOnLaunch: true
  routeTableRefs:
    - from:
        name: __NETWORK_NAME__-rtb-public
  tags:
    - key: Name
      value: __NETWORK_NAME__-subnet-public-a
---
apiVersion: ec2.services.k8s.aws/v1alpha1
kind: NATGateway
metadata:
  name: __NETWORK_NAME__-natgw
  namespace: __NAMESPACE__
  annotations:
    services.k8s.aws/region: __AWS_REGION__
spec:
  allocationRef:
    from:
      name: __NETWORK_NAME__-eip-nat
  subnetRef:
    from:
      name: __NETWORK_NAME__-subnet-public-a
  tags:
    - key: Name
      value: __NETWORK_NAME__-natgw
---
apiVersion: ec2.services.k8s.aws/v1alpha1
kind: RouteTable
metadata:
  name: __NETWORK_NAME__-rtb-private
  namespace: __NAMESPACE__
  annotations:
    services.k8s.aws/region: __AWS_REGION__
spec:
  vpcRef:
    from:
      name: __NETWORK_NAME__-vpc
  routes:
    - destinationCIDRBlock: 0.0.0.0/0
      natGatewayRef:
        from:
          name: __NETWORK_NAME__-natgw
  tags:
    - key: Name
      value: __NETWORK_NAME__-rtb-private
---
apiVersion: ec2.services.k8s.aws/v1alpha1
kind: Subnet
metadata:
  name: __NETWORK_NAME__-subnet-private-a
  namespace: __NAMESPACE__
  annotations:
    services.k8s.aws/region: __AWS_REGION__
spec:
  vpcRef:
    from:
      name: __NETWORK_NAME__-vpc
  cidrBlock: __PRIVATE_SUBNET_A_CIDR__
  availabilityZone: __PRIVATE_SUBNET_A_AZ__
  mapPublicIPOnLaunch: false
  routeTableRefs:
    - from:
        name: __NETWORK_NAME__-rtb-private
  tags:
    - key: Name
      value: __NETWORK_NAME__-subnet-private-a
---
apiVersion: ec2.services.k8s.aws/v1alpha1
kind: Subnet
metadata:
  name: __NETWORK_NAME__-subnet-private-b
  namespace: __NAMESPACE__
  annotations:
    services.k8s.aws/region: __AWS_REGION__
spec:
  vpcRef:
    from:
      name: __NETWORK_NAME__-vpc
  cidrBlock: __PRIVATE_SUBNET_B_CIDR__
  availabilityZone: __PRIVATE_SUBNET_B_AZ__
  mapPublicIPOnLaunch: false
  routeTableRefs:
    - from:
        name: __NETWORK_NAME__-rtb-private
  tags:
    - key: Name
      value: __NETWORK_NAME__-subnet-private-b
