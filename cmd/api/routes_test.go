package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Gyebran/GoWork/docs"
	"github.com/Gyebran/GoWork/internal/platform/httpx"
	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"
)

func TestOpenAPIRoutesMatchProductionRouter(t *testing.T) {
	router := newRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), httpx.NewReadiness(func(context.Context) error { return nil }), nil, nil)
	var contract struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(docs.Specification, &contract); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{}
	for path, item := range contract.Paths {
		for method := range item {
			if strings.Contains(" get post put patch delete head options trace ", " "+method+" ") {
				want[strings.ToUpper(method)+" "+path] = true
			}
		}
	}
	err := chi.Walk(router.(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		key := method + " " + route
		if !want[key] {
			t.Errorf("Undocumented route: %s", key)
		}
		delete(want, key)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for key := range want {
		t.Errorf("Unimplemented operation: %s", key)
	}
	for _, path := range []string{"/docs", "/openapi.yaml"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("X-Request-ID", "m11-docs")
		router.ServeHTTP(w, req)
		if w.Code != 200 || w.Header().Get("X-Request-ID") != "m11-docs" {
			t.Fatalf("%s: %d", path, w.Code)
		}
		if path == "/openapi.yaml" && !bytes.Equal(w.Body.Bytes(), docs.Specification) {
			t.Fatal("served spec differs")
		}
		if path == "/docs" && !strings.Contains(w.Body.String(), "SwaggerUIBundle") {
			t.Fatal("missing UI")
		}
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("POST", "/docs", nil))
	if w.Code != 405 || w.Header().Get("Allow") != "GET" {
		t.Fatal("docs method contract")
	}
}
