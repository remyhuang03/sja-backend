// Package localize provides the shared API and report message catalogs.
package localize

import (
	"embed"
	"encoding/json"
	"net/http"
	"strings"
)

//go:embed *.json
var catalogs embed.FS
var messages = map[string]map[string]string{}

func init() {
	for _, locale := range []string{"en", "ja"} {
		data, err := catalogs.ReadFile(locale + ".json")
		if err != nil {
			panic(err)
		}
		var catalog map[string]string
		if err := json.Unmarshal(data, &catalog); err != nil {
			panic(err)
		}
		messages[locale] = catalog
	}
}

// Locale gives an explicit site preference precedence over the API language header.
func Locale(r *http.Request) string {
	if cookie, err := r.Cookie("sja_locale"); err == nil {
		switch cookie.Value {
		case "zh", "en", "ja":
			return cookie.Value
		}
	}
	// Keep the same Chinese default as the frontend; API clients can opt in explicitly.
	switch strings.ToLower(r.Header.Get("X-SJA-Locale")) {
	case "en", "en-us", "en-gb":
		return "en"
	case "ja", "ja-jp":
		return "ja"
	}
	return "zh"
}

func Text(locale, message string) string {
	if locale == "zh" {
		return message
	}
	if translated, ok := messages[locale][message]; ok {
		return translated
	}
	for prefix, translations := range map[string][2]string{
		"缺少文件 ":   {"Missing file: ", "ファイルがありません: "},
		"缺少图片 ":   {"Missing image: ", "画像がありません: "},
		"积木数量超过 ": {"Block count exceeds ", "ブロック数が上限を超えています: "},
		"原作品：":    {"Original project: ", "元の作品: "},
		"待比较作品：":  {"Compared project: ", "比較する作品: "},
	} {
		if strings.HasPrefix(message, prefix) {
			i := 0
			if locale == "ja" {
				i = 1
			}
			return translations[i] + Text(locale, strings.TrimPrefix(message, prefix))
		}
	}
	return message
}
