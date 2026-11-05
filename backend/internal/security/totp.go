package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"time"
)

func TOTPSecret() string {
	data := make([]byte, 20)
	if _, err := rand.Read(data); err != nil {
		panic(err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(data)
}
func totpCode(secret string, counter int64) string {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return ""
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(counter))
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	digest := mac.Sum(nil)
	offset := digest[len(digest)-1] & 15
	number := binary.BigEndian.Uint32(digest[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", number%1000000)
}

// TOTPCounter accepts a one-step clock skew. Callers must persist the counter to reject replay.
func TOTPCounter(secret, code string, at time.Time) (int64, bool) {
	if len(code) != 6 {
		return 0, false
	}
	counter := at.Unix() / 30
	for _, delta := range []int64{0, -1, 1} {
		if Equal(totpCode(secret, counter+delta), code) {
			return counter + delta, true
		}
	}
	return 0, false
}
