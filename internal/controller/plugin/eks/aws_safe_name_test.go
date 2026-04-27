package eks

import "testing"

func TestSafeFargateProfileBaseName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"reserved prefix is renamed", "eks-local", "aioc-eks-local"},
		{"non-reserved cluster is unchanged", "pmc-local", "pmc-local"},
		{"multi-segment eks-prefix", "eks-prod-us-1", "aioc-eks-prod-us-1"},
		{"trivial name", "my-cluster", "my-cluster"},
		{"aws-eks- prefix is NOT eks- prefix", "aws-eks-test", "aws-eks-test"},
		{"empty stays empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := safeFargateProfileBaseName(tc.in)
			if got != tc.want {
				t.Errorf("safeFargateProfileBaseName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
