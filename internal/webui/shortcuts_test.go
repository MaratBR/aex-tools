package webui

import (
	"reflect"
	"testing"

	"aex/internal/tool"
)

func TestToolsAPI(t *testing.T) {
	a := &App{host: Host{Tools: func() []tool.Tool {
		return []tool.Tool{
			{Name: "quota", Summary: "Quota"},
			{Name: "secret", Hidden: true},
			{Name: "cm-release", Sub: []tool.Tool{{Name: "pull-all", Summary: "Pull"}}},
		}
	}}}
	got, err := toolsAPI(a, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []ShortcutTool{{Line: "quota", Summary: "Quota"}, {Line: "cm-release pull-all", Summary: "Pull"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
