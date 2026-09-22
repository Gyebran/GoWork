package users

import (
	"errors"
	"testing"
)

func TestFilters(t *testing.T) {
	for _, raw := range []string{"", "page=2&limit=5&role=TECHNICIAN&is_active=false"} {
		if _, err := ParseFilter(raw); err != nil {
			t.Fatal(raw, err)
		}
	}
	for _, raw := range []string{"page=0", "limit=101", "page=100001", "page=+1", "page=1&page=2", "role=admin", "is_active=True", "role=", "unknown=x", "page=x", "page=999999999999999999999999", "x=%zz"} {
		if _, err := ParseFilter(raw); err == nil {
			t.Fatal("accepted", raw)
		}
	}
}
func TestDeactivationGuard(t *testing.T) {
	for _, c := range []struct {
		self   bool
		role   string
		n      int64
		orders bool
		want   error
	}{{true, "ADMIN", 2, false, SelfDeactivation}, {false, "ADMIN", 1, false, LastAdmin}, {false, "TECHNICIAN", 2, true, ActiveOrders}, {false, "ADMIN", 2, false, nil}, {false, "TECHNICIAN", 1, false, nil}} {
		if err := GuardDeactivation(c.self, c.role, c.n, c.orders); !errors.Is(err, c.want) {
			t.Fatal(err, c.want)
		}
	}
}

func TestUUIDValidation(t *testing.T) {
	for _, raw := range []string{"00000000x0000x0000x0000x000000000001", "00000000000000000000000000000001", "invalid"} {
		if _, err := ID(raw); err == nil {
			t.Fatal("accepted malformed UUID", raw)
		}
	}
	if _, err := ID("00000000-0000-0000-0000-000000000001"); err != nil {
		t.Fatal(err)
	}
}

func TestNameRejectsNUL(t *testing.T) {
	if validName("name\x00suffix") {
		t.Fatal("NUL accepted")
	}
}
