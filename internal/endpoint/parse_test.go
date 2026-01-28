package endpoint

import (
	"strings"
	"testing"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    clusterv1.APIEndpoint
		wantErr bool
	}{
		{
			name:  "host only defaults port to 443",
			input: "example.com",
			want:  clusterv1.APIEndpoint{Host: "example.com", Port: 443},
		},
		{
			name:  "host port is preserved",
			input: "example.com:6443",
			want:  clusterv1.APIEndpoint{Host: "example.com", Port: 6443},
		},
		{
			name:  "https url defaults port to 443",
			input: "https://example.com",
			want:  clusterv1.APIEndpoint{Host: "example.com", Port: 443},
		},
		{
			name:  "https url trailing slash is accepted",
			input: "https://example.com/",
			want:  clusterv1.APIEndpoint{Host: "example.com", Port: 443},
		},
		{
			name:  "https url host port is preserved",
			input: "https://example.com:6443",
			want:  clusterv1.APIEndpoint{Host: "example.com", Port: 6443},
		},
		{
			name:  "leading and trailing whitespace is ignored",
			input: "  https://example.com:6443  ",
			want:  clusterv1.APIEndpoint{Host: "example.com", Port: 6443},
		},
		{
			name:    "empty string returns error",
			input:   "",
			wantErr: true,
		},
		{
			name:    "http scheme is rejected",
			input:   "http://example.com",
			wantErr: true,
		},
		{
			name:    "userinfo is rejected",
			input:   "https://user:pass@example.com",
			wantErr: true,
		},
		{
			name:    "invalid port returns error",
			input:   "example.com:abc",
			wantErr: true,
		},
		{
			name:    "empty port returns error",
			input:   "example.com:",
			wantErr: true,
		},
		{
			name:    "url path is rejected",
			input:   "https://example.com/path",
			wantErr: true,
		},
		{
			name:    "url query is rejected",
			input:   "https://example.com?x=y",
			wantErr: true,
		},
		{
			name:    "url fragment is rejected",
			input:   "https://example.com#frag",
			wantErr: true,
		},
		{
			name:    "port 0 is rejected",
			input:   "example.com:0",
			wantErr: true,
		},
		{
			name:    "port above 65535 is rejected",
			input:   "example.com:65536",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}

			if got != tt.want {
				t.Fatalf("Parse returned %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParse_ErrorDoesNotLeakRawEndpoint(t *testing.T) {
	t.Parallel()

	input := "user:pass@example.com/%zz"
	candidate := "https://" + input

	_, err := Parse(input)
	if err == nil {
		t.Fatalf("expected error")
	}

	if strings.Contains(err.Error(), input) {
		t.Fatalf("error %q contains raw input %q", err.Error(), input)
	}
	if strings.Contains(err.Error(), candidate) {
		t.Fatalf("error %q contains raw candidate %q", err.Error(), candidate)
	}
	if strings.Contains(err.Error(), "user:pass") {
		t.Fatalf("error %q contains credentials", err.Error())
	}
}
