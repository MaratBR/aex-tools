package autostart

import (
	"slices"
	"testing"
	"time"
)

func TestParseDays(t *testing.T) {
	for _, c := range []struct {
		text string
		want []string
	}{
		{"", nil},
		{"Fri, mon ,mon", []string{"mon", "fri"}},
		{"workdays,sun", []string{"mon", "tue", "wed", "thu", "fri", "sun"}},
		{"every-day", Week},
	} {
		got, err := ParseDays(c.text)
		if err != nil || !slices.Equal(got, c.want) {
			t.Errorf("ParseDays(%q) = %v, %v; want %v", c.text, got, err, c.want)
		}
	}
	if _, err := ParseDays("mon,funday"); err == nil {
		t.Error("ParseDays(funday): no error")
	}
}

func TestDayName(t *testing.T) {
	if dayName(time.Sunday) != "sun" || dayName(time.Monday) != "mon" || dayName(time.Saturday) != "sat" {
		t.Error("dayName maps weekdays wrong")
	}
}
