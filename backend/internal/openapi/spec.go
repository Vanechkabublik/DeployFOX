package openapi

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.json
var spec []byte

func Serve(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write(spec)
}
