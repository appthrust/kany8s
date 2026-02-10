package eks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *EKSKarpenterBootstrapperReconciler) cleanupOrphanCNINetworkInterfacesForNodeSecurityGroup(
	ctx context.Context,
	owner *clusterv1.Cluster,
	region string,
) (done bool, deletedCount int, remainingCount int, retErr error) {
	if r == nil || owner == nil {
		return true, 0, 0, nil
	}

	region = strings.TrimSpace(region)
	if region == "" {
		return true, 0, 0, nil
	}

	// We only operate on the bootstrapper-managed node SecurityGroup, which is created
	// only when vpc-node-security-group-ids is empty.
	sgCRName := shortenAWSName(fmt.Sprintf("%s-karpenter-node-sg", owner.Name), 63)
	sg := &unstructured.Unstructured{}
	sg.SetGroupVersionKind(ackSecurityGroupGVK)
	if err := r.Get(ctx, client.ObjectKey{Namespace: owner.Namespace, Name: sgCRName}, sg); err != nil {
		if apierrors.IsNotFound(err) || isNoMatchError(err) {
			return true, 0, 0, nil
		}
		return false, 0, 0, err
	}
	if !isManagedByBootstrapper(sg.GetAnnotations()) {
		return true, 0, 0, nil
	}

	sgID, _ := readNestedString(sg.Object, "status", "id")
	sgID = strings.TrimSpace(sgID)
	if sgID == "" {
		return true, 0, 0, nil
	}

	return cleanupOrphanCNINetworkInterfaces(ctx, region, sgID)
}

func cleanupOrphanCNINetworkInterfaces(ctx context.Context, region, securityGroupID string) (done bool, deletedCount int, remainingCount int, retErr error) {
	region = strings.TrimSpace(region)
	securityGroupID = strings.TrimSpace(securityGroupID)
	if region == "" {
		return false, 0, 0, fmt.Errorf("region is empty")
	}
	if securityGroupID == "" {
		return false, 0, 0, fmt.Errorf("securityGroupID is empty")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return false, 0, 0, fmt.Errorf("load AWS config: %w", err)
	}
	ec2c := ec2.NewFromConfig(cfg)

	groupID := "group-id"
	input := &ec2.DescribeNetworkInterfacesInput{Filters: []ec2types.Filter{{Name: &groupID, Values: []string{securityGroupID}}}}

	candidates := []string{}
	p := ec2.NewDescribeNetworkInterfacesPaginator(ec2c, input)
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return false, 0, 0, fmt.Errorf("describe network interfaces: %w", err)
		}
		for _, ni := range page.NetworkInterfaces {
			if !isOrphanCNINetworkInterface(ni) {
				continue
			}
			id := strings.TrimSpace(derefString(ni.NetworkInterfaceId))
			if id == "" {
				continue
			}
			candidates = append(candidates, id)
		}
	}

	if len(candidates) == 0 {
		return true, 0, 0, nil
	}

	for _, id := range candidates {
		id := strings.TrimSpace(id)
		if id == "" {
			continue
		}
		_, err := ec2c.DeleteNetworkInterface(ctx, &ec2.DeleteNetworkInterfaceInput{NetworkInterfaceId: &id})
		if err != nil {
			if ignoreInvalidNetworkInterfaceNotFound(err) {
				continue
			}
			return false, deletedCount, len(candidates), fmt.Errorf("delete network interface %q: %w", id, err)
		}
		deletedCount++
	}

	// Cleanup is in progress.
	return false, deletedCount, len(candidates), nil
}

func isOrphanCNINetworkInterface(ni ec2types.NetworkInterface) bool {
	if ni.Attachment != nil {
		return false
	}
	if ni.Status != ec2types.NetworkInterfaceStatusAvailable {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(tagValue(ni.TagSet, "eks:eni:owner")), "amazon-vpc-cni") {
		return false
	}
	return true
}

func tagValue(tags []ec2types.Tag, key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	for _, t := range tags {
		if strings.TrimSpace(derefString(t.Key)) != key {
			continue
		}
		return strings.TrimSpace(derefString(t.Value))
	}
	return ""
}

func ignoreInvalidNetworkInterfaceNotFound(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode() == "InvalidNetworkInterfaceID.NotFound"
	}
	return false
}
