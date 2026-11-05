package security

import (
	"encoding/base32"
	"testing"
	"time"
)

func TestRFC6238(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	for _, tc := range []struct {
		at   int64
		code string
	}{{59, "287082"}, {1111111109, "081804"}, {1111111111, "050471"}, {1234567890, "005924"}, {2000000000, "279037"}} {
		counter, valid := TOTPCounter(secret, tc.code, time.Unix(tc.at, 0))
		if !valid || counter != tc.at/30 {
			t.Errorf("RFC vector %d failed", tc.at)
		}
	}
	if _, valid := TOTPCounter(secret, "000000", time.Unix(59, 0)); valid {
		t.Fatal("invalid code accepted")
	}
}
