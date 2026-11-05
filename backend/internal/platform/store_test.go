package platform

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestBuildLogAcceptsBinaryOutputAndSplitUnicode(t *testing.T) {
	input := strings.Repeat("a", 4095) + "я\x00\xff"
	got := cleanLogLine(input)
	if !utf8.ValidString(got) || strings.ContainsRune(got, 0) || len(got) > 4096 {
		t.Fatal("log cannot be safely stored in PostgreSQL")
	}
	if cleanLogLine("привет\x00\xff") != "привет" {
		t.Fatal("valid Unicode was lost")
	}
}
