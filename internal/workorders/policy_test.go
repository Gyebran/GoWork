package workorders

import "testing"

func TestLifecycleMatrix(t *testing.T) {
	allowed := map[string]bool{"OPEN/CANCELLED": true, "ASSIGNED/IN_PROGRESS": true, "ASSIGNED/CANCELLED": true, "IN_PROGRESS/COMPLETED": true, "IN_PROGRESS/CANCELLED": true}
	for _, from := range []string{"OPEN", "ASSIGNED", "IN_PROGRESS", "COMPLETED", "CANCELLED"} {
		for _, to := range []string{"OPEN", "ASSIGNED", "IN_PROGRESS", "COMPLETED", "CANCELLED"} {
			if (Transition(from, to) == nil) != allowed[from+"/"+to] {
				t.Fatal(from, to)
			}
		}
	}
}
func TestFilters(t *testing.T) {
	for _, raw := range []string{"page=0", "limit=101", "status=unknown", "priority=urgent", "assigned_to=bad", "asset_id=00000000x0000x0000x0000x000000000001", "status=OPEN&status=ASSIGNED", "search=x"} {
		if _, err := ParseFilter(raw); err == nil {
			t.Fatal(raw)
		}
	}
	if _, err := ParseFilter("status=OPEN&priority=HIGH&page=2&limit=5"); err != nil {
		t.Fatal(err)
	}
}
