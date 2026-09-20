package rbac

import "testing"

func TestOwnership(t *testing.T) {
	for _, c := range []struct {
		role, actor, assigned string
		want                  bool
	}{{"ADMIN", "a", "b", true}, {"MANAGER", "a", "b", true}, {"TECHNICIAN", "a", "a", true}, {"TECHNICIAN", "a", "b", false}, {"TECHNICIAN", "a", "", false}, {"TECHNICIAN", "", "", false}, {"UNKNOWN", "a", "a", false}} {
		if AssignedScope(c.role, c.actor, c.assigned) != c.want {
			t.Fatal(c)
		}
	}
	if !AssignmentFilterAllowed("TECHNICIAN", "a", "") || !AssignmentFilterAllowed("TECHNICIAN", "a", "a") || AssignmentFilterAllowed("TECHNICIAN", "a", "b") {
		t.Fatal("scope widening")
	}
}
