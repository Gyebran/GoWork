package config

import "testing"

func TestMilestone10IntentionalFailure(t *testing.T) {
	t.Fatal("Milestone 10 isolated CI gate demonstration: intentional test failure")
}
