// A plugin with a widget and settings, for widget_test.go.
package main

import (
	"encoding/json"
	"errors"
	"os"

	"aex/internal/plugin"
	"aex/internal/tool"
)

func main() {
	var access []plugin.Option
	if os.Getenv("WIDGET_TEST_ACCESS") != "" {
		access = append(access, plugin.Jira)
	}
	plugin.Main(tool.Tool{Name: "widgetplugin", Summary: "test", Run: func([]string) error { return nil }},
		append(access, plugin.Widget{
			ID: "hello", Name: "Hello", Summary: "says hello", W: 2, H: 1, HTML: "<p>hello</p>",
			Calls: map[string]func(json.RawMessage) (any, error){
				"echo": func(args json.RawMessage) (any, error) { return args, nil },
				"fail": func(json.RawMessage) (any, error) { return nil, errors.New("it failed") },
			},
		}, plugin.Settings{Summary: "test settings", Run: func([]string) error { return nil }})...)
}
