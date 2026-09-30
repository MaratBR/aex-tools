package webui

import "testing"

func TestAbout(t *testing.T) {
	info, err := (&App{}).About()
	if err != nil {
		t.Fatal(err)
	}
	if info.Build.Version == "" || info.Build.Repo == "" || info.Licenses.License != "Apache-2.0" || len(info.Licenses.ThirdParty) == 0 {
		t.Errorf("about = %+v", info.Build)
	}
}
