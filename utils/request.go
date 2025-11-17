package utils

import (
	"encoding/json"
	"net/http"
)

// DecodeJSONRequest decodes JSON from HTTP request body into the provided interface.
// Usage: var data MyType; if err := DecodeJSONRequest(r, &data); err != nil { ... }
func DecodeJSONRequest(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

// Deprecated: Use DecodeJSONRequest instead - same functionality, better name
func DecodeJSON(r *http.Request, v interface{}) error {
	return DecodeJSONRequest(r, v)
}
