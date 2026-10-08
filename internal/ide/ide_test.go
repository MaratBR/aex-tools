package ide

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRecentProjects(t *testing.T) {
	dir := t.TempDir()
	sln := filepath.Join(dir, "App.sln")
	os.WriteFile(sln, nil, 0o600)
	xml := `<application>
  <component name="RiderRecentProjectsManager">
    <option name="additionalInfo">
      <map>
        <entry key="` + filepath.ToSlash(sln) + `">
          <value>
            <RecentProjectMetaInfo opened="true">
              <option name="activationTimestamp" value="1700000005000" />
              <option name="projectOpenTimestamp" value="1700000000000" />
            </RecentProjectMetaInfo>
          </value>
        </entry>
        <entry key="` + filepath.ToSlash(filepath.Join(dir, "gone.sln")) + `"><value><RecentProjectMetaInfo /></value></entry>
        <entry key="` + filepath.ToSlash(dir) + `"><value><RecentProjectMetaInfo /></value></entry>
      </map>
    </option>
    <option name="lastOpenedProject" value="` + filepath.ToSlash(dir) + `/other" />
  </component>
</application>`
	got := recentProjects(strings.NewReader(xml))
	if len(got) != 2 {
		t.Fatalf("%+v", got)
	}
	if got[0].Path != sln || got[0].Kind != "solution" || !got[0].Opened.Equal(time.UnixMilli(1700000005000)) {
		t.Errorf("%+v", got[0])
	}
	if got[1].Path != dir || got[1].Kind != "folder" {
		t.Errorf("%+v", got[1])
	}
}

func TestVSRecent(t *testing.T) {
	dir := t.TempDir()
	sln := filepath.Join(dir, "App.sln")
	os.WriteFile(sln, nil, 0o600)
	js := `[{"Key":"` + strings.ReplaceAll(sln, `\`, `\\`) + `","Value":{"LocalProperties":{"FullPath":"` + strings.ReplaceAll(sln, `\`, `\\`) + `","Type":0},"LastAccessed":"2024-05-01T10:00:00.0000000+00:00"}},` +
		`{"Key":"C:\\gone\\x.sln","Value":{"LocalProperties":{"FullPath":"C:\\gone\\x.sln"}}}]`
	xml := `<content><indexed><collection name="Other"><value name="value">[]</value></collection>` +
		`<collection name="CodeContainers.Offline"><value name="value">` + strings.ReplaceAll(js, `"`, "&quot;") + `</value></collection></indexed></content>`
	got := vsRecent(strings.NewReader(xml))
	if len(got) != 1 || got[0].Path != sln || got[0].Opened.Year() != 2024 {
		t.Fatalf("%+v", got)
	}
}

func TestFileURIPath(t *testing.T) {
	want := "/home/me/src app"
	in := "file:///home/me/src%20app"
	if runtime.GOOS == "windows" {
		want, in = `C:\src app`, "file:///c%3A/src%20app"
	}
	if got := fileURIPath(in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := fileURIPath("vscode-remote://wsl+Ubuntu/home"); got != "" {
		t.Errorf("remote: %q", got)
	}
}

func TestCompareVersions(t *testing.T) {
	if compareVersions("Rider2024.10", "Rider2024.9") <= 0 || compareVersions("17.9.1", "17.12.0") >= 0 {
		t.Error("wrong order")
	}
	if got := newest([]string{"GoLand 2024.3", "GoLand 2025.1", "GoLand 2024.10"}); got != "GoLand 2025.1" {
		t.Error(got)
	}
}

func TestKnown(t *testing.T) {
	for id, want := range map[string]string{"rider": "Rider", "vscode": "Visual Studio Code", "vs": "Visual Studio", "vs2022": "Visual Studio 2022", "vs2022-2": "Visual Studio 2022"} {
		if got, ok := Known(id); !ok || got != want {
			t.Errorf("%s: %q %v", id, got, ok)
		}
	}
	if _, ok := Known("notepad"); ok {
		t.Error("notepad is known")
	}
}
