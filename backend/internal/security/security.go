package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"path"
	"regexp"
	"strings"

	"golang.org/x/crypto/argon2"
)

func Token() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func ID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
func Hash(value string) string { h := sha256.Sum256([]byte(value)); return hex.EncodeToString(h[:]) }
func Equal(a, b string) bool   { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

func Password(password string) string {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		panic(err)
	}
	hash := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return base64.RawStdEncoding.EncodeToString(salt) + "." + base64.RawStdEncoding.EncodeToString(hash)
}

func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, ".")
	if len(parts) != 2 {
		return false
	}
	salt, e1 := base64.RawStdEncoding.DecodeString(parts[0])
	want, e2 := base64.RawStdEncoding.DecodeString(parts[1])
	if e1 != nil || e2 != nil || len(salt) != 16 || len(want) != 32 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}

func Encrypt(key []byte, value string) (string, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	g, err := cipher.NewGCM(b)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, g.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(g.Seal(nonce, nonce, []byte(value), []byte("deployfox:v1"))), nil
}

func Decrypt(key []byte, value string) (string, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	g, err := cipher.NewGCM(b)
	if err != nil {
		return "", err
	}
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(data) < g.NonceSize() {
		return "", errors.New("invalid encrypted value")
	}
	plain, err := g.Open(nil, data[:g.NonceSize()], data[g.NonceSize():], []byte("deployfox:v1"))
	return string(plain), err
}

func Webhook(secret string, body []byte, signature string) bool {
	if secret == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal([]byte("sha256="+hex.EncodeToString(mac.Sum(nil))), []byte(signature))
}

var domainPattern = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
var ImagePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/:@-]{0,254}$`)
var EnvPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

func Domain(s string) bool {
	return len(s) <= 253 && domainPattern.MatchString(s) && !strings.HasSuffix(s, ".localhost") && !strings.HasSuffix(s, ".local")
}
func RelativePath(s string) bool {
	return s != "" && !strings.ContainsAny(s, "\\\x00\r\n") && !strings.HasPrefix(s, "/") && path.Clean(s) != ".." && !strings.HasPrefix(path.Clean(s), "../")
}

// Repository imports deliberately allow only GitHub HTTPS. DNS is checked again by the agent.
func Repository(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("only https://github.com/owner/repository imports are supported")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.Contains(u.Path, "..") {
		return errors.New("invalid GitHub repository")
	}
	return nil
}

func PublicIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	address = address.Unmap()
	for _, prefix := range reservedRanges {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

var reservedRanges = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("2001:db8::/32"),
}
