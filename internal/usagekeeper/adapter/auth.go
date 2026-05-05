package adapter

import (
	"net"
	"net/http"
)

type ManagementKeyVerifier func(clientIP string, localClient bool, provided string) (allowed bool, statusCode int, message string)

func LocalClientFromRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	ip := net.ParseIP(ClientIPFromRequest(r))
	return ip != nil && ip.IsLoopback()
}

func ClientIPFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
