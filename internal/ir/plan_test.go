package ir

import "testing"

func TestAMessageIsFoundByItsKey(t *testing.T) {
	plan := &Plan{Messages: []string{"nav.home", "time.days_ago"}}
	index, ok := plan.MessageIndex("time.days_ago")
	if !ok || index != 1 {
		t.Errorf("index = %d, ok = %v", index, ok)
	}
	if _, ok := plan.MessageIndex("nope"); ok {
		t.Error("an unknown key is missing")
	}
}
