package plugin

import (
	"errors"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"aex/internal/secrets"
	"aex/internal/settings"
)

// buildWidgetPlugin builds testdata/widgetplugin into dir.
func buildWidgetPlugin(t *testing.T, dir string) Info {
	t.Helper()
	name := "widgetplugin"
	file := name
	if runtime.GOOS == "windows" {
		file += ".exe"
	}
	path := filepath.Join(dir, file)
	if out, err := exec.Command("go", "build", "-o", path, "./testdata/widgetplugin").CombinedOutput(); err != nil {
		t.Fatalf("building the test plugin: %v\n%s", err, out)
	}
	return Info{Name: name, Path: path}
}

func approveFile(t *testing.T, path string) string {
	t.Helper()
	o, err := openPlugin(path)
	if err != nil {
		t.Fatal(err)
	}
	defer o.close()
	settings.Credentials.Set(hashKey(path), o.hash)
	return o.hash
}

func TestWidgets(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a plugin")
	}
	dir := t.TempDir()
	settings.Credentials, _ = secrets.Open("file", dir, filepath.Join(dir, "credentials.json"))
	settings.DataDir = dir
	p := buildWidgetPlugin(t, dir)

	// Nothing runs before the plugin is approved.
	_, err := WidgetPage(p, "hello")
	if we, ok := errors.AsType[*WidgetError](err); !ok || we.Plugin != p.Name {
		t.Fatalf("not approved: err = %v, want a WidgetError", err)
	}
	hash := approveFile(t, p.Path)

	info, err := inspect(p.Name, p.Path)
	want := []WidgetInfo{
		{ID: "hello", Name: "Hello", Summary: "says hello", W: 2, H: 1, Access: []Access{}},
		{ID: "jira", Name: "Jira", W: 1, H: 1, Access: []Access{Jira}},
	}
	if err != nil || !reflect.DeepEqual(info.Widgets, want) {
		t.Fatalf("described widgets = %+v, %v", info.Widgets, err)
	}
	if info.Settings != "test settings" {
		t.Errorf("described settings = %q, want %q", info.Settings, "test settings")
	}
	if !reflect.DeepEqual(info.Provides, []string{GitRepos}) {
		t.Errorf("described provides = %q", info.Provides)
	}
	if got, err := Provided(info, GitRepos); err != nil || string(got) != `["a","b"]` {
		t.Errorf("provided %s = %s, %v", GitRepos, got, err)
	}
	if _, err := Provided(info, "nope"); err == nil {
		t.Error("provided something not described: no error")
	}
	if page, err := WidgetPage(p, "hello"); page != "<p>hello</p>" || err != nil {
		t.Fatalf("page = %q, %v", page, err)
	}
	if _, err := WidgetPage(p, "nope"); err == nil {
		t.Fatal("page of an unknown widget: no error")
	}

	got, err := WidgetCall(p, "hello", "echo", []byte(`{"a":1}`))
	if err != nil || string(got) != `{"a":1}` {
		t.Fatalf("echo = %s, %v", got, err)
	}
	if _, err := WidgetCall(p, "hello", "fail", nil); err == nil || err.Error() != "it failed" {
		t.Fatalf("fail: err = %v, want it failed", err)
	}
	if _, err := WidgetCall(p, "hello", "nope", nil); err == nil {
		t.Fatal("unknown call: no error")
	}

	// A widget cannot need access its plugin does not ask for.
	if _, err := WidgetCall(p, "jira", "echo", nil); err == nil {
		t.Fatal("widget needing access the plugin does not ask for: no error")
	}

	// Access the widget needs must be granted already: a widget cannot ask.
	t.Setenv("WIDGET_TEST_ACCESS", "1")
	widgetMu.Lock()
	clear(widgetDescribed)
	widgetMu.Unlock()
	// A widget needing none of it works without it.
	if got, err := WidgetCall(p, "hello", "echo", []byte(`{"a":1}`)); err != nil || string(got) != `{"a":1}` {
		t.Fatalf("widget needing no access: echo = %s, %v", got, err)
	}
	_, err = WidgetCall(p, "jira", "echo", nil)
	if we, ok := errors.AsType[*WidgetError](err); !ok {
		t.Fatalf("access not granted: err = %v, want a WidgetError", err)
	} else if want := "plugin widgetplugin has not been granted access to jira"; we.Error() != want {
		t.Fatalf("err = %q, want %q", we.Error(), want)
	}
	settings.Credentials.Set(grantKey(p.Path), hash+" jira")
	// Granted, but its settings are not set (none are here): still not asked for.
	_, err = WidgetCall(p, "jira", "echo", nil)
	if we, ok := errors.AsType[*WidgetError](err); !ok || we.Reason != "needs JIRA_EMAIL, which is not set" {
		t.Fatalf("settings not set: err = %v", err)
	}

	// A plugin built before widgets listed their access (none listed): its widgets need all of it.
	settings.Credentials.Delete(grantKey(p.Path))
	widgetMu.Lock()
	widgetDescribed[hash] = description{Access: []Access{Jira}, Widgets: []WidgetInfo{{ID: "hello"}}}
	widgetMu.Unlock()
	if _, err := WidgetCall(p, "hello", "echo", nil); !errors.As(err, new(*WidgetError)) {
		t.Fatalf("widget of an older plugin, access not granted: err = %v, want a WidgetError", err)
	}
}
