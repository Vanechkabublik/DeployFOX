package domains

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"os"
	"time"

	"deployfox/internal/auth"
	"deployfox/internal/platform"
	"deployfox/internal/security"
	"github.com/go-chi/chi/v5"
)

type Handler struct{ Store *platform.Store }

func (h Handler) Check(w http.ResponseWriter, r *http.Request) {
	p, err := h.Store.Project(r.Context(), chi.URLParam(r, "id"))
	if err != nil || p.Domain == "" {
		auth.Error(w, 404, "Project domain not set")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	addresses, err := net.DefaultResolver.LookupHost(ctx, p.Domain)
	result := map[string]any{"domain": p.Domain, "addresses": addresses, "dns_matches": false, "certificate_valid": false, "port_80": false, "port_443": false}
	if err != nil {
		result["error"] = "DNS resolution failed"
		auth.JSON(w, 200, result)
		return
	}
	expected := net.ParseIP(os.Getenv("SERVER_IP"))
	if !security.PublicIP(expected) {
		result["error"] = "Set SERVER_IP to the public VPS IP in .env to verify DNS and HTTPS"
		auth.JSON(w, 200, result)
		return
	}
	result["expected_ip"] = expected.String()
	matches := false
	for _, address := range addresses {
		if expected.Equal(net.ParseIP(address)) {
			matches = true
		}
	}
	result["dns_matches"] = matches
	if !matches {
		result["error"] = "A/AAAA record does not include the configured VPS IP"
		auth.JSON(w, 200, result)
		return
	}
	// Dial the configured public IP, not an untrusted/rebound domain address.
	dialer := net.Dialer{Timeout: 3 * time.Second}
	for _, port := range []string{"80", "443"} {
		conn, e := dialer.DialContext(ctx, "tcp", net.JoinHostPort(expected.String(), port))
		if e == nil {
			result["port_"+port] = true
			conn.Close()
		}
	}
	tcp, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(expected.String(), "443"))
	if err != nil {
		result["error"] = "Public port 443 is unreachable from the server"
		auth.JSON(w, 200, result)
		return
	}
	conn := tls.Client(tcp, &tls.Config{ServerName: p.Domain, MinVersion: tls.VersionTLS12})
	defer conn.Close()
	if err = conn.HandshakeContext(ctx); err != nil {
		result["error"] = "TLS certificate handshake/verification failed"
		auth.JSON(w, 200, result)
		return
	}
	result["certificate_valid"] = true
	if certs := conn.ConnectionState().PeerCertificates; len(certs) > 0 {
		result["certificate_expires"] = certs[0].NotAfter
	}
	auth.JSON(w, 200, result)
}
