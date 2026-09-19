package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"unicode/utf8"
)

const MaxBodyBytes = 1 << 20

// DecodeStringObject is for flat string DTOs such as login, not arbitrary JSON.
func DecodeStringObject(w http.ResponseWriter, r *http.Request, allowed ...string) (map[string]string, bool) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		WriteError(w, r, 415, "UNSUPPORTED_MEDIA_TYPE", "Use application/json")
		return nil, false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxBodyBytes))
	if err != nil {
		var max *http.MaxBytesError
		if errors.As(err, &max) {
			WriteError(w, r, 413, "PAYLOAD_TOO_LARGE", "Request body exceeds 1 MiB")
		} else {
			WriteError(w, r, 400, "INVALID_JSON", "Invalid JSON body")
		}
		return nil, false
	}
	bad := func() (map[string]string, bool) {
		WriteError(w, r, 400, "INVALID_JSON", "Invalid JSON object")
		return nil, false
	}
	if !utf8.Valid(body) {
		return bad()
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return bad()
	}
	values := map[string]string{}
	seen := map[string]bool{}
	invalidFields := false
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return bad()
		}
		key, ok := keyToken.(string)
		if !ok || seen[key] {
			return bad()
		}
		seen[key] = true
		var raw json.RawMessage
		if dec.Decode(&raw) != nil {
			return bad()
		}
		known := false
		for _, a := range allowed {
			if a == key {
				known = true
			}
		}
		if !known {
			invalidFields = true
			continue
		}
		if bytes.Equal(raw, []byte("null")) {
			invalidFields = true
			continue
		}
		var value string
		if json.Unmarshal(raw, &value) != nil {
			return bad()
		}
		values[key] = value
	}
	tok, err = dec.Token()
	if err != nil || tok != json.Delim('}') {
		return bad()
	}
	if _, err = dec.Token(); err != io.EOF {
		return bad()
	}
	if invalidFields {
		WriteError(w, r, 422, "VALIDATION_ERROR", "Unknown or null fields are not allowed")
		return nil, false
	}
	return values, true
}
