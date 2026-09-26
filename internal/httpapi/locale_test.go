package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestLocalizedErrors(t *testing.T) {
	handler := New(nil, t.TempDir(), "test-key").Handler()
	for _, tc := range []struct{ locale, want string }{
		{"zh-Hant", "審核金鑰無效"}, {"zh", "审核密钥无效"}, {"en", "Invalid review key."}, {"ja", "審査キーが無効です。"},
	} {
		r := httptest.NewRequest("GET", "/api/v2/project-display-review", nil)
		r.Header.Set("Cookie", "sja_locale="+tc.locale)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		var response map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if w.Code != 401 || response["message"] != tc.want || response["msg"] != tc.want {
			t.Fatalf("%s: %d %s", tc.locale, w.Code, w.Body)
		}
	}
}
