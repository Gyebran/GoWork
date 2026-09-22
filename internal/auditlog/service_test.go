package auditlog

import (
	"strings"
	"testing"
)

func TestFilters(t *testing.T) {
	for _, s := range []string{"", "entity_type=user&action=USER_UPDATED&limit=2&page=3", "entity_type=asset&entity_id=00000000-0000-0000-0000-000000000001"} {
		if _, err := ParseFilter(s); err != nil {
			t.Fatal(s, err)
		}
	}
	for _, s := range []string{"entity_id=00000000-0000-0000-0000-000000000001", "actor_id=null", "actor_id=00000000x0000x0000x0000x000000000001", "page=0", "limit=101", "page=100001", "page=1&page=2", "action=LOGIN", "entity_type=User", "sort=id", "action=", "page=%zz"} {
		if _, err := ParseFilter(s); err == nil {
			t.Fatal("accepted", s)
		}
	}
}
func TestSanitization(t *testing.T) {
	for _, kind := range []string{"user", "asset", "work_order"} {
		raw, err := Sanitize(kind, []byte(`{"id":"public","password":"secret","password_hash":"secret","access_token":"secret","nested":{"token":"secret"}}`))
		if err != nil || string(raw) != `{"id":"public"}` {
			t.Fatal(kind, string(raw), err)
		}
	}
	raw, err := Sanitize("user", []byte(`{"is_active":true,"password_changed":true,"password":"secret"}`))
	if err != nil || !strings.Contains(string(raw), `"password_changed":true`) || strings.Contains(string(raw), "secret") {
		t.Fatal(string(raw), err)
	}
	for _, raw := range []string{`[]`, `"bad"`, `{"name":{"password":"secret"}}`, `{"is_active":"true"}`, `{"name":null}`} {
		if _, err := Sanitize("user", []byte(raw)); err == nil {
			t.Fatal("unsafe snapshot", raw)
		}
	}
	for _, raw := range [][]byte{nil, []byte("null")} {
		got, err := Sanitize("user", raw)
		if err != nil || got != nil {
			t.Fatal(got, err)
		}
	}
	got, err := Sanitize("work_order", []byte(`{"assigned_to":null,"completed_at":null}`))
	if err != nil || !strings.Contains(string(got), `"assigned_to":null`) {
		t.Fatal(string(got), err)
	}
}
