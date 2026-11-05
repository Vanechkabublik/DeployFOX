package docker

import "strings"

func transientBuildOutput(output string) bool {
	text := strings.ToLower(output)
	for _, marker := range []string{"i/o timeout", "tls handshake timeout", "connection reset by peer", "temporary failure in name resolution", "unexpected eof", "service unavailable", "toomanyrequests"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
