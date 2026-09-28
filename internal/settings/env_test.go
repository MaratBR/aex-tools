package settings

import "testing"

func TestParseEnv(t *testing.T) {
	got := ParseEnv("# comment\r\nA=1\nexport B = two # note\nC='x # y'\nD=\"l1\\nl2\"\nbad line\n E=\n")
	want := map[string]string{"A": "1", "B": "two", "C": "x # y", "D": "l1\nl2", "E": ""}
	if len(got) != len(want) {
		t.Fatalf("ParseEnv = %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestSetLineRoundTrip(t *testing.T) {
	text := "# header\nA=1\nB=2\n"
	for _, v := range []string{"plain@x.com", "has space", "a#b", "tok+/=:"} {
		value := v
		out, err := setLine(text, "A", &value)
		if err != nil {
			t.Fatal(err)
		}
		if got := ParseEnv(out)["A"]; got != v {
			t.Errorf("round trip %q: got %q from %q", v, got, out)
		}
	}
	out, _ := setLine(text, "A", nil)
	if out != "# header\nB=2\n" {
		t.Errorf("remove: %q", out)
	}
	value := "3"
	if out, _ := setLine(text, "C", &value); out != text+"C=3\n" {
		t.Errorf("append: %q", out)
	}
	quote := "it's"
	if _, err := setLine(text, "A", &quote); err == nil {
		t.Error("single quote accepted")
	}
}

func TestProblem(t *testing.T) {
	for value, ok := range map[string]bool{"8": true, " 7.5 ": true, "0": false, "25": false, "x": false, "": false} {
		if (Problem("HOURS_PER_DAY", value) == "") != ok {
			t.Errorf("HOURS_PER_DAY %q valid = %v", value, !ok)
		}
	}
	for value, ok := range map[string]bool{"5.5": true, "-12": true, "5.75": true, "5.1": false, "15": false} {
		if (Problem("TZ_OFFSET_HOURS", value) == "") != ok {
			t.Errorf("TZ_OFFSET_HOURS %q valid = %v", value, !ok)
		}
	}
}
