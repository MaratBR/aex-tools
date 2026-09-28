package main

import (
	"slices"
	"testing"
)

func TestNaturalLess(t *testing.T) {
	keys := []string{"CM-10", "AB-2", "CM-9", "CM-100", "CM-9"}
	slices.SortFunc(keys, func(a, b string) int {
		switch {
		case naturalLess(a, b):
			return -1
		case naturalLess(b, a):
			return 1
		}
		return 0
	})
	if want := []string{"AB-2", "CM-9", "CM-9", "CM-10", "CM-100"}; !slices.Equal(keys, want) {
		t.Errorf("sorted = %v", keys)
	}
}

func TestSplitArgs(t *testing.T) {
	got := splitArgs(`1 --range "last week"  --manual`)
	if want := []string{"1", "--range", "last week", "--manual"}; !slices.Equal(got, want) {
		t.Errorf("splitArgs = %q", got)
	}
}
