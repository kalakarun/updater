package release

import (
	"strings"
	"testing"
)

func TestVersions(t *testing.T) {
	version, err := ParseInstalled([]byte(" 58826\n"))
	if err != nil || version.String() != "5.88.26" {
		t.Fatal(version, err)
	}
	next, err := ParseVersion("5.89.00")
	if err != nil || next != 58900 || next <= version {
		t.Fatal(next, err)
	}
	for _, old := range []string{"58826", "58826\n", "58826\r\n"} {
		if string(next.Installed([]byte(old))) != strings.Replace(old, "58826", "58900", 1) {
			t.Fatal(old)
		}
	}
	for _, invalid := range []string{"", "-1", "5.88.26", "123abc", "9999", "1000000", "999999999999999999999", "1e5"} {
		if _, err := ParseInstalled([]byte(invalid)); err == nil {
			t.Errorf("accepted swversion %q", invalid)
		}
	}
	for _, invalid := range []string{"0.1.1", "100.1.1", "5.100.0", "5.0.100", "05.1.1", "5.1", "5.1.1.1", "5.-1.1", "5.1.1 ", "5.1e1.1"} {
		if _, err := ParseVersion(invalid); err == nil {
			t.Errorf("accepted version %q", invalid)
		}
	}
}
