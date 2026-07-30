package archive

import (
	"testing"
)

func TestParseTitle_extractsTitle(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		html string
		want string
	}{
		{"simple", `<html><head><title>Hello World</title></head></html>`, "Hello World"},
		{"whitespace", `<title>  Trim Me  </title>`, "Trim Me"},
		{"uppercase tag", `<TITLE>UPPER</TITLE>`, "UPPER"},
		{"mixed case", `<TiTlE>Mixed</TiTlE>`, "Mixed"},
		{"attributes", `<title id="x">With Attrs</title>`, "With Attrs"},
		{"none", `<html><body>no title here</body></html>`, ""},
		{"empty", `<title></title>`, ""},
		{"no close", `<title>unclosed`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ParseTitle([]byte(tc.html)); got != tc.want {
				t.Errorf("ParseTitle(%q) = %q, want %q", tc.html, got, tc.want)
			}
		})
	}
}
