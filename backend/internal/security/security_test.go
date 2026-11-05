package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"strings"
	"testing"
)

func TestPassword(t *testing.T) {
	hash := Password("correct horse battery staple")
	if !VerifyPassword("correct horse battery staple", hash) || VerifyPassword("wrong", hash) {
		t.Fatal("password verification failed")
	}
	if hash == Password("correct horse battery staple") {
		t.Fatal("salt reused")
	}
}

func TestPublicIPBoundaries(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.1.2.3", "169.254.169.254", "100.64.0.1", "203.0.113.10", "255.255.255.255", "::1", "fc00::1", "2001:db8::1"} {
		if PublicIP(net.ParseIP(raw)) {
			t.Errorf("non-public IP accepted: %s", raw)
		}
	}
	if !PublicIP(net.ParseIP("1.1.1.1")) {
		t.Fatal("public address rejected")
	}
}
func TestAuthenticatedEncryption(t *testing.T) {
	key := make([]byte, 32)
	cipher, err := Encrypt(key, "super-secret")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(cipher, "super-secret") {
		t.Fatal("plaintext stored")
	}
	value, err := Decrypt(key, cipher)
	if err != nil || value != "super-secret" {
		t.Fatal("roundtrip failed", err)
	}
	wrong := make([]byte, 32)
	wrong[0] = 1
	if _, err = Decrypt(wrong, cipher); err == nil {
		t.Fatal("wrong key accepted")
	}
	if _, err = Decrypt(key, "broken"); err == nil {
		t.Fatal("corruption accepted")
	}
}
func TestWebhookSignature(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main"}`)
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !Webhook("secret", body, sig) || Webhook("wrong", body, sig) || Webhook("", body, sig) || Webhook("secret", []byte("tampered"), sig) {
		t.Fatal("signature verification failed")
	}
}
func TestImportBoundaries(t *testing.T) {
	for _, raw := range []string{"http://169.254.169.254/latest/meta-data", "file:///etc/passwd", "https://github.com.evil.test/o/r", "https://a:b@github.com/o/r", "https://github.com/o/r?redirect=1", "https://localhost/o/r"} {
		if Repository(raw) == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	if Repository("https://github.com/owner/project.git") != nil {
		t.Fatal("valid repository rejected")
	}
	for _, p := range []string{"../secret", "/etc/passwd", "x/../../etc", "C:\\Windows", "x\x00"} {
		if RelativePath(p) {
			t.Errorf("accepted path %q", p)
		}
	}
	if !RelativePath(".") || !RelativePath("app/Dockerfile") {
		t.Fatal("valid path rejected")
	}
}
