package analyzer

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

const sample = `{"extensions":[],"targets":[{"isStage":false,"costumes":[{}],"sounds":[],"blocks":{"hat":{"opcode":"event_whenflagclicked","topLevel":true,"next":"move"},"move":{"opcode":"motion_movesteps","next":"hat","inputs":{"STEPS":[3,[12,"x","id"],[4,10]]}},"unused":{"opcode":"looks_say","topLevel":true},"shadow":{"opcode":"math_number","shadow":true}}}]}`

func TestAnalyzeCyclesAndInlineVariables(t *testing.T) {
	p, err := Read([]byte(sample), "test.json")
	if err != nil {
		t.Fatal(err)
	}
	r, err := Analyze(p, len(sample))
	if err != nil {
		t.Fatal(err)
	}
	if r.TotalBlockCount != 4 || r.ValidBlockCount != 3 || r.TotalParagraphCount != 2 || r.ValidParagraphCount != 1 || r.CategoryCount["data"] != 1 {
		t.Fatalf("unexpected report: %+v", r)
	}
	again, _ := Analyze(p, len(sample))
	if again.TotalBlockCount != r.TotalBlockCount {
		t.Fatal("analysis mutated project")
	}
}
func TestArchiveValidation(t *testing.T) {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, _ := w.Create("../project.json")
	f.Write([]byte(sample))
	w.Close()
	if _, err := Read(b.Bytes(), "unsafe.sb3"); err == nil {
		t.Fatal("accepted missing root project.json")
	}
	b.Reset()
	w = zip.NewWriter(&b)
	f, _ = w.Create("project.json")
	f.Write([]byte(sample))
	w.Close()
	if _, err := Read(b.Bytes(), "valid.sb3"); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{`{}`, `null`, `{"targets":"bad"}`, `not json`} {
		if _, err := Read([]byte(input), "x.json"); err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
}
func TestComparisonAndEscape(t *testing.T) {
	a, _ := Read([]byte(sample), "x.json")
	c, err := Compare(a, a, 1, 1)
	if err != nil || c.Similarity != 1 {
		t.Fatalf("identity: %+v %v", c, err)
	}
	var b Project
	json.Unmarshal([]byte(strings.ReplaceAll(sample, "motion_movesteps", "sound_play")), &b)
	c, err = Compare(a, &b, 1, 1)
	if err != nil || c.Similarity >= 1 {
		t.Fatalf("changed project: %+v", c)
	}
	r := Report{CategoryCount: map[string]int{"<script>alert(1)</script>": 1}, TotalBlockCount: 1}
	svg := string(SVG(r, "desc", "top12"))
	if strings.Contains(svg, "<script>") {
		t.Fatal("unescaped SVG")
	}
}
func TestConcurrentIndependentReports(t *testing.T) {
	for i := 0; i < 20; i++ {
		t.Run("analysis", func(t *testing.T) {
			t.Parallel()
			p, _ := Read([]byte(sample), "x.json")
			r, err := Analyze(p, 1)
			if err != nil || r.TotalBlockCount != 4 {
				t.Fatal(r, err)
			}
		})
	}
}

func TestLocalizedSVG(t *testing.T) {
	report := Report{CategoryCount: map[string]int{"motion": 2}, TotalBlockCount: 2}
	for _, tc := range []struct{ locale, title, label string }{
		{"en", "SJA Project Analysis", "Motion"},
		{"ja", "SJA 作品分析レポート", "動き"},
		{"zh", "SJA 作品分析报告", "运动"},
		{"zh-Hant", "SJA 作品分析報告", "運動"},
	} {
		svg := string(SVG(report, "desc", "top12", tc.locale))
		if !strings.Contains(svg, tc.title) || !strings.Contains(svg, tc.label) {
			t.Fatalf("Missing %s translations: %s", tc.locale, svg)
		}
	}
}
