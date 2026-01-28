package endpoint

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func Parse(raw string) (clusterv1.APIEndpoint, error) {
	endpoint := strings.TrimSpace(raw)
	if endpoint == "" {
		return clusterv1.APIEndpoint{}, fmt.Errorf("endpoint is empty")
	}

	candidate := endpoint
	hasScheme := strings.Contains(candidate, "://")
	if !hasScheme {
		candidate = "https://" + candidate
	}

	u, err := url.Parse(candidate)
	if err != nil {
		// net/url includes the raw URL string in its errors (e.g. `parse "<url>": ...`).
		// Avoid leaking credentials or other sensitive data by returning only the underlying error.
		var uerr *url.Error
		if errors.As(err, &uerr) {
			return clusterv1.APIEndpoint{}, fmt.Errorf("parse endpoint: %v", uerr.Err)
		}
		return clusterv1.APIEndpoint{}, fmt.Errorf("parse endpoint: %v", err)
	}

	if u.Scheme != "https" {
		return clusterv1.APIEndpoint{}, fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	if u.User != nil {
		return clusterv1.APIEndpoint{}, fmt.Errorf("userinfo is not supported")
	}
	if u.RawQuery != "" {
		return clusterv1.APIEndpoint{}, fmt.Errorf("query is not supported")
	}
	if u.Fragment != "" {
		return clusterv1.APIEndpoint{}, fmt.Errorf("fragment is not supported")
	}
	if u.Path != "" && u.Path != "/" {
		return clusterv1.APIEndpoint{}, fmt.Errorf("path is not supported")
	}
	if u.Host == "" {
		return clusterv1.APIEndpoint{}, fmt.Errorf("host is empty")
	}
	if strings.HasSuffix(u.Host, ":") {
		return clusterv1.APIEndpoint{}, fmt.Errorf("port is empty")
	}

	host := u.Hostname()
	if host == "" {
		return clusterv1.APIEndpoint{}, fmt.Errorf("host is empty")
	}

	port := 443
	if portStr := u.Port(); portStr != "" {
		portInt, err := strconv.Atoi(portStr)
		if err != nil {
			return clusterv1.APIEndpoint{}, fmt.Errorf("invalid port %q", portStr)
		}
		if portInt < 1 || portInt > 65535 {
			return clusterv1.APIEndpoint{}, fmt.Errorf("invalid port %d", portInt)
		}
		port = portInt
	}

	return clusterv1.APIEndpoint{Host: host, Port: int32(port)}, nil
}
