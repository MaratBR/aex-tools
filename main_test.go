package main

import (
	"slices"
	"testing"
)

func TestSplitArgs(t *testing.T) {
	got := splitArgs(`1 --range "last week"  --manual`)
	if want := []string{"1", "--range", "last week", "--manual"}; !slices.Equal(got, want) {
		t.Errorf("splitArgs = %q", got)
	}
}
