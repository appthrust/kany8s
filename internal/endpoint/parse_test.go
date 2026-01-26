package endpoint

import (
	"testing"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    clusterv1.APIEndpoint
		wantErr bool
	}{
		{
			name: "https without port defaults to 443",
			in:   "https://example.com",
			want: clusterv1.APIEndpoint{Host: "example.com", Port: 443},
		},
		{
			name: "https with port",
			in:   "https://example.com:6443",
			want: clusterv1.APIEndpoint{Host: "example.com", Port: 6443},
		},
		{
			name: "host without port defaults to 443",
			in:   "example.com",
			want: clusterv1.APIEndpoint{Host: "example.com", Port: 443},
		},
		{
			name: "host with port",
			in:   "example.com:6443",
			want: clusterv1.APIEndpoint{Host: "example.com", Port: 6443},
		},
		{
			name: "trims whitespace",
			in:   "  https://example.com:6443  ",
			want: clusterv1.APIEndpoint{Host: "example.com", Port: 6443},
		},
		{
			name: "ipv6 https without port defaults to 443",
			in:   "https://[2001:db8::1]",
			want: clusterv1.APIEndpoint{Host: "2001:db8::1", Port: 443},
		},
		{
			name: "ipv6 https with port",
			in:   "https://[2001:db8::1]:6443",
			want: clusterv1.APIEndpoint{Host: "2001:db8::1", Port: 6443},
		},
		{
			name: "ipv6 host without port defaults to 443",
			in:   "[2001:db8::1]",
			want: clusterv1.APIEndpoint{Host: "2001:db8::1", Port: 443},
		},
		{
			name: "ipv6 host with port",
			in:   "[2001:db8::1]:6443",
			want: clusterv1.APIEndpoint{Host: "2001:db8::1", Port: 6443},
		},
		{
			name:    "empty",
			in:      "",
			wantErr: true,
		},
		{
			name:    "non-https scheme",
			in:      "http://example.com",
			wantErr: true,
		},
		{
			name:    "invalid port",
			in:      "example.com:99999",
			wantErr: true,
		},
		{
			name:    "path not allowed",
			in:      "https://example.com/foo",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := Parse(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse() error=%v wantErr=%v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got != tt.want {
				t.Fatalf("Parse() got=%+v want=%+v", got, tt.want)
			}
		})
	}
}
