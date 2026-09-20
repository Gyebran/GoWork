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

// DecodeObject validates flat typed DTOs before domain validation.
func DecodeObject(w http.ResponseWriter, r *http.Request, fields map[string]string) (map[string]json.RawMessage, bool) {
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
	bad := func() (map[string]json.RawMessage, bool) {
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
	values := map[string]json.RawMessage{}
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
		kind, known := fields[key]
		if !known {
			invalidFields = true
			continue
		}
		if bytes.Equal(raw, []byte("null")) {
			invalidFields = true
			continue
		}
		var value any
		switch kind {
		case "string":
			value = new(string)
		case "bool":
			value = new(bool)
		default:
			return bad()
		}
		if json.Unmarshal(raw, value) != nil {
			return bad()
		}
		values[key] = raw
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

// DecodeStringObject preserves the login DTO contract.
func DecodeStringObject(w http.ResponseWriter, r *http.Request, allowed ...string) (map[string]string, bool) {
	fields := map[string]string{}
	for _, key := range allowed {
		fields[key] = "string"
	}
	raw, ok := DecodeObject(w, r, fields)
	if !ok {
		return nil, false
	}
	values := map[string]string{}
	for key, v := range raw {
		var s string
		_ = json.Unmarshal(v, &s)
		values[key] = s
	}
	return values, true
}
