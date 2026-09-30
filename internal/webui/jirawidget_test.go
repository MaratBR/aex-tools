package webui

import (
	"testing"
	"time"
)

func TestTicketsJQL(t *testing.T) {
	for _, c := range []struct {
		args map[string]any
		want string // "" when it fails
	}{
		{map[string]any{"mode": "assigned"}, "assignee = currentUser() AND statusCategory != Done ORDER BY updated DESC"},
		{map[string]any{"mode": "field", "fields": []any{"customfield_10050", "customfield_7"}},
			"(cf[10050] = currentUser() OR cf[7] = currentUser()) AND statusCategory != Done ORDER BY updated DESC"},
		{map[string]any{"mode": "field", "fields": []any{}}, ""},
		{map[string]any{"mode": "field", "fields": []any{"assignee"}}, ""},
		{map[string]any{"mode": "field", "fields": []any{"customfield_1) OR (1=1"}}, ""},
		{map[string]any{"mode": "jql", "jql": "project = CM ORDER BY created"}, "project = CM ORDER BY created"},
		{map[string]any{"mode": "jql", "jql": "  "}, ""},
		{map[string]any{"mode": "other"}, ""},
	} {
		got, err := ticketsJQL(c.args)
		if c.want == "" {
			if err == nil {
				t.Errorf("%v: got %q, want an error", c.args, got)
			}
		} else if err != nil || got != c.want {
			t.Errorf("%v: got %q, %v; want %q", c.args, got, err, c.want)
		}
	}
}

func TestTicketsMax(t *testing.T) {
	for _, c := range []struct {
		max  any
		want int
	}{{nil, 20}, {float64(5), 5}, {float64(0), 1}, {float64(500), 100}, {"7", 20}} {
		if got := ticketsMax(map[string]any{"max": c.max}); got != c.want {
			t.Errorf("max %v: got %d, want %d", c.max, got, c.want)
		}
	}
}

func TestNewHours(t *testing.T) {
	monday := time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC)
	if got := newHours(monday); got != 72 {
		t.Errorf("Monday: got %d, want 72", got)
	}
	if got := newHours(monday.AddDate(0, 0, 1)); got != 24 {
		t.Errorf("Tuesday: got %d, want 24", got)
	}
}
