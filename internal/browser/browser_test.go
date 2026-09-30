package browser

import "testing"

func TestParse(t *testing.T) {
	for _, c := range []struct{ link, address, browser string }{
		{"https://example.com/a?b=1", "https://example.com/a?b=1", ""},
		{"aex+brave://google.com", "https://google.com", "brave"},
		{"AEX+Firefox://http://intranet/x", "http://intranet/x", "firefox"},
		{"aex+yandex://https://ya.ru", "https://ya.ru", "yandex"},
	} {
		address, browser, err := Parse(c.link)
		if err != nil || address != c.address || browser != c.browser {
			t.Errorf("Parse(%q) = %q, %q, %v; want %q, %q", c.link, address, browser, err, c.address, c.browser)
		}
	}
	for _, bad := range []string{"file:///C:/x", "javascript:alert(1)", "aex+brave:google.com", "aex+://x", "https://"} {
		if _, _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) took it", bad)
		}
	}
}

func TestFind(t *testing.T) {
	if b := Find("Google-Chrome"); b == nil || b.Name != "chrome" {
		t.Fatalf("Find alias = %v", b)
	}
	if Find("netscape") != nil {
		t.Fatal("found an unknown browser")
	}
}
