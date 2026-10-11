package compiler

import (
	"testing"
	"time"
)

func TestFreshnessBudgetMatchesFeedSchedule(t *testing.T) {
	now := time.Date(2026, 10, 12, 12, 7, 0, 0, time.UTC)
	for _, test := range []struct {
		schedule string
		interval time.Duration
	}{{"@hourly", time.Hour}, {"@daily", 24 * time.Hour}, {"0 */4 * * *", 4 * time.Hour}} {
		budget, err := FreshnessBudget(test.schedule, now, 15*time.Minute, 5*time.Minute, 2*time.Minute)
		if err != nil || budget != test.interval+22*time.Minute {
			t.Fatalf("%s: %s %v", test.schedule, budget, err)
		}
	}
	if _, err := FreshnessBudget("invalid", now, 0, 0, 0); err == nil {
		t.Fatal("invalid schedule accepted")
	}
}
