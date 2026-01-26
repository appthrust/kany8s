package endpoint

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
)

const defaultHTTPSPort int32 = 443

// Parse parses an endpoint string in one of the following formats:
// - https://host[:port]
// - host[:port]
//
// If port is omitted, it defaults to 443.
func Parse(raw string) (clusterv1.APIEndpoint, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return clusterv1.APIEndpoint{}, fmt.Errorf("endpoint is empty")
	}

	if !strings.Contains(raw, "://") {
		host, port, err := parseHostPort(raw)
		if err != nil {
			return clusterv1.APIEndpoint{}, err
		}
		return clusterv1.APIEndpoint{Host: host, Port: port}, nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return clusterv1.APIEndpoint{}, fmt.Errorf("parse endpoint %q: %w", raw, err)
	}
	if u.Scheme != "https" {
		return clusterv1.APIEndpoint{}, fmt.Errorf("unsupported scheme %q (expected https)", u.Scheme)
	}
	if u.User != nil {
		return clusterv1.APIEndpoint{}, fmt.Errorf("endpoint must not include userinfo")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return clusterv1.APIEndpoint{}, fmt.Errorf("endpoint must not include query or fragment")
	}
	if u.Path != "" && u.Path != "/" {
		return clusterv1.APIEndpoint{}, fmt.Errorf("endpoint must not include path")
	}

	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return clusterv1.APIEndpoint{}, fmt.Errorf("endpoint host is empty")
	}

	port := defaultHTTPSPort
	if portStr := strings.TrimSpace(u.Port()); portStr != "" {
		p, err := parsePort(portStr)
		if err != nil {
			return clusterv1.APIEndpoint{}, err
		}
		port = p
	}

	return clusterv1.APIEndpoint{Host: host, Port: port}, nil
}

func parseHostPort(raw string) (string, int32, error) {
	if strings.ContainsAny(raw, " /?#") {
		return "", 0, fmt.Errorf("endpoint must be host[:port] without path/query/fragment")
	}

	// IPv6 in brackets.
	if strings.HasPrefix(raw, "[") {
		if strings.HasSuffix(raw, "]") {
			host := strings.TrimSuffix(strings.TrimPrefix(raw, "["), "]")
			host = strings.TrimSpace(host)
			if host == "" {
				return "", 0, fmt.Errorf("endpoint host is empty")
			}
			return host, defaultHTTPSPort, nil
		}

		host, portStr, err := net.SplitHostPort(raw)
		if err != nil {
			return "", 0, fmt.Errorf("parse endpoint %q: %w", raw, err)
		}
		p, err := parsePort(portStr)
		if err != nil {
			return "", 0, err
		}
		host = strings.TrimSpace(host)
		if host == "" {
			return "", 0, fmt.Errorf("endpoint host is empty")
		}
		return host, p, nil
	}

	colonCount := strings.Count(raw, ":")
	switch colonCount {
	case 0:
		host := strings.TrimSpace(raw)
		if host == "" {
			return "", 0, fmt.Errorf("endpoint host is empty")
		}
		return host, defaultHTTPSPort, nil
	case 1:
		host, portStr, err := net.SplitHostPort(raw)
		if err != nil {
			return "", 0, fmt.Errorf("parse endpoint %q: %w", raw, err)
		}
		host = strings.TrimSpace(host)
		if host == "" {
			return "", 0, fmt.Errorf("endpoint host is empty")
		}
		p, err := parsePort(portStr)
		if err != nil {
			return "", 0, err
		}
		return host, p, nil
	default:
		// Likely an IPv6 literal without port.
		host := strings.TrimSpace(raw)
		if host == "" {
			return "", 0, fmt.Errorf("endpoint host is empty")
		}
		return host, defaultHTTPSPort, nil
	}
}

func parsePort(raw string) (int32, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("endpoint port is empty")
	}
	p64, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid port %q: %w", raw, err)
	}
	if p64 < 1 || p64 > 65535 {
		return 0, fmt.Errorf("port %d out of range", p64)
	}
	return int32(p64), nil
}
