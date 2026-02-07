package eks

import (
	"context"
	"fmt"
	"strings"
	"time"

	authenticatorToken "sigs.k8s.io/aws-iam-authenticator/pkg/token"
)

type TokenGenerator interface {
	Generate(ctx context.Context, region string, clusterName string) (string, time.Time, error)
}

type AWSIAMAuthenticatorTokenGenerator struct {
	gen authenticatorToken.Generator
}

func NewAWSIAMAuthenticatorTokenGenerator() (*AWSIAMAuthenticatorTokenGenerator, error) {
	gen, err := authenticatorToken.NewGenerator(false, false)
	if err != nil {
		return nil, fmt.Errorf("create aws-iam-authenticator token generator: %w", err)
	}
	return &AWSIAMAuthenticatorTokenGenerator{gen: gen}, nil
}

func (g *AWSIAMAuthenticatorTokenGenerator) Generate(ctx context.Context, region string, clusterName string) (string, time.Time, error) {
	if g == nil || g.gen == nil {
		return "", time.Time{}, fmt.Errorf("token generator is not configured")
	}
	cluster := strings.TrimSpace(clusterName)
	if cluster == "" {
		return "", time.Time{}, fmt.Errorf("cluster name is required")
	}

	tok, err := g.gen.GetWithOptions(ctx, &authenticatorToken.GetTokenOptions{
		Region:    strings.TrimSpace(region),
		ClusterID: cluster,
	})
	if err != nil {
		return "", time.Time{}, err
	}

	if tok.Token == "" {
		return "", time.Time{}, fmt.Errorf("generated token is empty")
	}
	return tok.Token, tok.Expiration.UTC(), nil
}
