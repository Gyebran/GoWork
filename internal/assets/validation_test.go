package assets

import "testing"

func TestFilters(t *testing.T) {
	for _, raw := range []string{"", "status=ACTIVE&category=HVAC&search=%25&page=2&limit=5"} {
		if _, err := ParseFilter(raw); err != nil {
			t.Fatal(raw, err)
		}
	}
	for _, raw := range []string{"status=active", "category=", "search=+", "assigned_to=someone", "page=0", "limit=101", "page=100001", "page=1&page=2", "sort=name", "search=%zz"} {
		if _, err := ParseFilter(raw); err == nil {
			t.Fatal("accepted", raw)
		}
	}
	for s, want := range map[string]string{"": "", "100%": "%100\\%%", "a_b": "%a\\_b%", "a\\b": "%a\\\\b%"} {
		if got := LiteralPattern(s); got != want {
			t.Fatal(got, want)
		}
	}
}
