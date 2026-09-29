package main

import (
	"strings"
	"testing"
)

func TestTags(t *testing.T) {
	for _, c := range []struct {
		summary     string
		mobile, web bool
	}{
		{"[Mobile] Fix login", true, false},
		{"[mobile][web] Shared banner", true, true},
		{"[MobileAPI] New endpoint", false, true},
		{"[Mobile] [MobileAPI] Push tokens", true, true},
		{"[Web] Dashboard", false, true},
		{"Mobile-friendly table", false, false},
	} {
		mobile, web := tags(c.summary)
		if mobile != c.mobile || web != c.web {
			t.Errorf("tags(%q) = %v, %v, want %v, %v", c.summary, mobile, web, c.mobile, c.web)
		}
	}
}

func TestHandoffComment(t *testing.T) {
	cfg := defaultConfig()
	mentions := func(qa person, cc []person) []string {
		var ids []string
		for _, n := range handoffComment(qa, cc)["content"].([]any)[0].(map[string]any)["content"].([]any) {
			if node := n.(map[string]any); node["type"] == "mention" {
				ids = append(ids, node["attrs"].(map[string]any)["text"].(string))
			}
		}
		return ids
	}
	for _, c := range []struct {
		qa   person
		cc   []person
		want string
	}{
		{cfg.DefaultQA, cfg.Cc, "@Heriberto Barajas @Alexander Martynets"}, // QA left out of Cc
		{person{"Q", "q"}, cfg.Cc, "@Q @Alexander Martynets @Heriberto Barajas"},
		{person{"Q", "q"}, append(cfg.Cc, cfg.Cc[0]), "@Q @Alexander Martynets @Heriberto Barajas"},
		{person{"Q", "q"}, nil, "@Q"},
	} {
		if got := strings.Join(mentions(c.qa, c.cc), " "); got != c.want {
			t.Errorf("handoffComment(%s) mentions %q, want %q", c.qa.Name, got, c.want)
		}
	}
}

func TestDefaultConfigIsValid(t *testing.T) {
	cfg := defaultConfig()
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
	cfg.Excluded[0].Mode = "maybe"
	if cfg.validate() == nil {
		t.Error("bad mode passed validation")
	}
}

func TestEpicInput(t *testing.T) {
	for _, c := range []struct {
		in, key    string
		number, ok bool
	}{
		{"1234", "CM-1234", true, true},
		{" cm-7042 ", "CM-7042", false, true},
		{"https://clearmechanic.atlassian.net/browse/CM-7042", "CM-7042", false, true},
		{"12a", "", false, false},
		{"", "", false, false},
	} {
		key, number, ok := epicInput(c.in, "CM")
		if ok != c.ok || (ok && (key != c.key || number != c.number)) {
			t.Errorf("epicInput(%q) = %q, %v, %v; want %q, %v, %v", c.in, key, number, ok, c.key, c.number, c.ok)
		}
	}
}

func TestMentions(t *testing.T) {
	for _, c := range []struct {
		text, name string
		want       bool
	}{
		{"Release 5.12", "5.12", true},
		{"Release 5.12.", "5.12", true},
		{"(5.12) hotfixes", "5.12", true},
		{"Release 5.12.1", "5.12", false},
		{"Release 15.12", "5.12", false},
		{"Sprint 2026", "2026 Release", false},
		{"anything", "", false},
	} {
		if got := mentions(c.text, c.name); got != c.want {
			t.Errorf("mentions(%q, %q) = %v, want %v", c.text, c.name, got, c.want)
		}
	}
}
