package localize

import (
	"net/http/httptest"
	"testing"
)

func TestLocale(t *testing.T) {
	for _, tc := range []struct{ cookie, header, want string }{
		{"", "", "zh"}, {"", "en", "en"}, {"", "ja", "ja"},
		{"sja_locale=ja", "en", "ja"}, {"sja_locale=zh", "en", "zh"},
		{"sja_locale=invalid", "", "zh"},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Cookie", tc.cookie)
		r.Header.Set("X-SJA-Locale", tc.header)
		if got := Locale(r); got != tc.want {
			t.Errorf("Locale(%q,%q) = %q, want %q", tc.cookie, tc.header, got, tc.want)
		}
	}
}

func TestText(t *testing.T) {
	if got := Text("en", "原作品：作品缺少 targets"); got != "Original project: The project has no targets." {
		t.Fatal(got)
	}
	if got := Text("ja", "缺少文件 original"); got != "ファイルがありません: original" {
		t.Fatal(got)
	}
	for key := range messages["en"] {
		if messages["ja"][key] == "" {
			t.Errorf("Missing Japanese translation for %q", key)
		}
	}
	if got := Text("en", "CustomExtension"); got != "CustomExtension" {
		t.Fatal(got)
	}
}
