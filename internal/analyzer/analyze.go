package analyzer

import (
	"embed"
	"encoding/csv"
	"fmt"
	"html"
	"sort"
	"strings"
)

//go:embed data/*.csv
var catalog embed.FS
var blockTypes = map[string]string{}
var categories = map[string][]string{}

func init() {
	for name, dest := range map[string]bool{"blocks_release.csv": true, "category_report_format.csv": false} {
		f, err := catalog.Open("data/" + name)
		if err != nil {
			panic(err)
		}
		rows, err := csv.NewReader(f).ReadAll()
		f.Close()
		if err != nil {
			panic(err)
		}
		for _, row := range rows {
			if len(row) >= 3 {
				if dest {
					blockTypes[row[0]] = row[2]
				} else {
					categories[row[0]] = row[1:]
				}
			}
		}
	}
}

type Report struct {
	CoreVersion         string         `json:"core_version"`
	FileSize            int            `json:"file_size"`
	SpriteCount         int            `json:"sprite_count"`
	CostumeCount        int            `json:"costume_count"`
	SoundCount          int            `json:"sound_count"`
	ValidParagraphCount int            `json:"valid_paragraph_count"`
	TotalParagraphCount int            `json:"total_paragraph_count"`
	ValidBlockCount     int            `json:"valid_block_count"`
	TotalBlockCount     int            `json:"total_block_count"`
	CategoryCount       map[string]int `json:"category_count"`
}

func category(op string, extensions []string) string {
	if strings.Contains(op, "argument_reporter") || op == "ccw_hat_parameter" {
		return "procedures"
	}
	best := ""
	for k := range categories {
		if strings.HasPrefix(op, k) && len(k) > len(best) {
			best = k
		}
	}
	for _, k := range extensions {
		if strings.HasPrefix(op, k) && len(k) > len(best) {
			best = k
		}
	}
	if best != "" {
		return best
	}
	if i := strings.IndexByte(op, '_'); i >= 0 {
		return op[:i]
	}
	parts := strings.Split(op, ".")
	if len(parts) > 2 {
		return strings.Join(parts[:2], ".")
	}
	return op
}

func Analyze(p *Project, size int) (Report, error) {
	r := Report{CoreVersion: "go-analyze-1", FileSize: size, CategoryCount: map[string]int{}}
	for _, t := range p.Targets {
		if !t.IsStage {
			r.SpriteCount++
		}
		r.CostumeCount += len(t.Costumes)
		r.SoundCount += len(t.Sounds)
		blocks, floating, err := decodeBlocks(t)
		if err != nil {
			return r, err
		}
		r.TotalBlockCount += floating
		r.CategoryCount["data"] += floating
		// Traverse valid roots first; sets prevent cycles and shared inputs from double counting.
		valid := map[string]bool{}
		queue := []string{}
		for id, b := range blocks {
			if b.TopLevel && !b.Shadow {
				r.TotalParagraphCount++
				if strings.Contains(blockTypes[b.Opcode], "top") {
					r.ValidParagraphCount++
					queue = append(queue, id)
				}
			}
		}
		for len(queue) > 0 {
			id := queue[len(queue)-1]
			queue = queue[:len(queue)-1]
			if valid[id] {
				continue
			}
			b, ok := blocks[id]
			if !ok {
				continue
			}
			valid[id] = true
			refs, _ := references(b)
			for _, ref := range refs {
				if !valid[ref] {
					queue = append(queue, ref)
				}
			}
		}
		for id, b := range blocks {
			if !b.Shadow {
				r.TotalBlockCount++
				r.CategoryCount[category(b.Opcode, p.Extensions)]++
				if valid[id] {
					r.ValidBlockCount++
				}
			}
			_, n := references(b)
			r.TotalBlockCount += n
			r.CategoryCount["data"] += n
			if valid[id] {
				r.ValidBlockCount += n
			}
		}
	}
	return r, nil
}

// SVG escapes all project-controlled category names before embedding them.
func SVG(r Report, order, mode string) []byte {
	keys := []string{}
	if mode == "classic" {
		keys = []string{"motion", "looks", "sound", "event", "control", "sensing", "operator", "data", "procedures", "pen", "canvas"}
	} else {
		for k, n := range r.CategoryCount {
			if n > 0 {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
	}
	if order != "none" {
		sort.SliceStable(keys, func(i, j int) bool {
			if r.CategoryCount[keys[i]] == r.CategoryCount[keys[j]] {
				return keys[i] < keys[j]
			}
			if order == "asc" {
				return r.CategoryCount[keys[i]] < r.CategoryCount[keys[j]]
			}
			return r.CategoryCount[keys[i]] > r.CategoryCount[keys[j]]
		})
	}
	if len(keys) > 12 {
		keys = keys[:12]
	}
	height := 230 + len(keys)*30
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="720" height="%d" viewBox="0 0 720 %d" role="img"><title>SJA 作品分析报告</title><rect width="720" height="%d" rx="20" fill="#191c23"/><g font-family="sans-serif" fill="#f4f4f5"><text x="32" y="48" font-size="26" font-weight="bold">SJA 作品分析报告</text>`, height, height, height)
	fmt.Fprintf(&b, `<text x="32" y="84" font-size="14">%.2f MiB · %d 个角色 · %d 个造型 · %d 个声音</text>`, float64(r.FileSize)/(1<<20), r.SpriteCount, r.CostumeCount, r.SoundCount)
	fmt.Fprintf(&b, `<text x="32" y="126" font-size="19">积木 %d / 有效 %d</text><text x="360" y="126" font-size="19">脚本 %d / 有效 %d</text><text x="32" y="165" font-size="13" fill="#a1a1aa">分类统计 · %s</text>`, r.TotalBlockCount, r.ValidBlockCount, r.TotalParagraphCount, r.ValidParagraphCount, r.CoreVersion)
	for i, k := range keys {
		label := k
		if c, ok := categories[k]; ok {
			label = c[0]
		}
		y := 200 + i*30
		width := 0
		if r.TotalBlockCount > 0 {
			width = 440 * r.CategoryCount[k] / r.TotalBlockCount
		}
		fmt.Fprintf(&b, `<text x="32" y="%d" font-size="13">%s</text><rect x="170" y="%d" width="%d" height="14" rx="4" fill="#a78bfa"/><text x="650" y="%d" font-size="13">%d</text>`, y, html.EscapeString(label), y-12, width, y, r.CategoryCount[k])
	}
	b.WriteString(`</g></svg>`)
	return []byte(b.String())
}

type Comparison struct {
	Method              string  `json:"method"`
	Similarity          float64 `json:"similarity"`
	OpcodeSimilarity    float64 `json:"opcode_similarity"`
	StructureSimilarity float64 `json:"structure_similarity"`
	Left                Report  `json:"left"`
	Right               Report  `json:"right"`
}

func features(p *Project) (map[string]int, map[string]int, error) {
	ops, edges := map[string]int{}, map[string]int{}
	for _, t := range p.Targets {
		blocks, _, err := decodeBlocks(t)
		if err != nil {
			return nil, nil, err
		}
		for _, b := range blocks {
			if b.Shadow {
				continue
			}
			ops[b.Opcode]++
			refs, _ := references(b)
			for _, ref := range refs {
				if child, ok := blocks[ref]; ok && !child.Shadow {
					edges[b.Opcode+"\x00"+child.Opcode]++
				}
			}
		}
	}
	return ops, edges, nil
}
func dice(a, b map[string]int) float64 {
	total, common := 0, 0
	for k, v := range a {
		total += v
		common += min(v, b[k])
	}
	for _, v := range b {
		total += v
	}
	if total == 0 {
		return 1
	}
	return float64(2*common) / float64(total)
}
func Compare(a, b *Project, as, bs int) (Comparison, error) {
	c := Comparison{Method: "opcode-edge-dice-v1"}
	var err error
	c.Left, err = Analyze(a, as)
	if err != nil {
		return c, err
	}
	c.Right, err = Analyze(b, bs)
	if err != nil {
		return c, err
	}
	ao, ae, err := features(a)
	if err != nil {
		return c, err
	}
	bo, be, err := features(b)
	if err != nil {
		return c, err
	}
	c.OpcodeSimilarity = dice(ao, bo)
	c.StructureSimilarity = dice(ae, be)
	c.Similarity = (c.OpcodeSimilarity + c.StructureSimilarity) / 2
	if len(ae) == 0 && len(be) == 0 {
		c.Similarity = c.OpcodeSimilarity
	}
	return c, nil
}
