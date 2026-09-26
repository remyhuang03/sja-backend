package analyzer

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

const MaxFileSize = 48 << 20
const MaxJSONSize = 64 << 20
const MaxBlocks = 200000

type Project struct {
	Targets    []Target `json:"targets"`
	Extensions []string `json:"extensions"`
}
type Target struct {
	IsStage  bool                       `json:"isStage"`
	Name     string                     `json:"name"`
	Blocks   map[string]json.RawMessage `json:"blocks"`
	Costumes []json.RawMessage          `json:"costumes"`
	Sounds   []json.RawMessage          `json:"sounds"`
}
type Block struct {
	Opcode   string                     `json:"opcode"`
	Next     *string                    `json:"next"`
	TopLevel bool                       `json:"topLevel"`
	Shadow   bool                       `json:"shadow"`
	Inputs   map[string]json.RawMessage `json:"inputs"`
}

// Read reads only project.json from an archive. It never extracts asset paths.
func Read(data []byte, filename string) (*Project, error) {
	if len(data) > MaxFileSize {
		return nil, errors.New("文件超过 48 MiB")
	}
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".sb3", ".cc3":
		z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, errors.New("作品压缩包无效")
		}
		var project *zip.File
		for _, f := range z.File {
			if f.Name == "project.json" {
				if project != nil {
					return nil, errors.New("压缩包包含重复的 project.json")
				}
				project = f
			}
		}
		if project == nil {
			return nil, errors.New("压缩包缺少 project.json")
		}
		if project.UncompressedSize64 > MaxJSONSize {
			return nil, errors.New("解压后的 project.json 超过 64 MiB")
		}
		r, err := project.Open()
		if err != nil {
			return nil, err
		}
		defer r.Close()
		data, err = io.ReadAll(io.LimitReader(r, MaxJSONSize+1))
		if err != nil {
			return nil, errors.New("读取 project.json 失败")
		}
		if len(data) > MaxJSONSize {
			return nil, errors.New("解压后的 project.json 超过 64 MiB")
		}
	case ".json":
	default:
		return nil, errors.New("仅支持 .sb3、.cc3 和 .json")
	}
	var p Project
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, errors.New("不是有效的 Scratch 3 JSON 文件")
	}
	if p.Targets == nil {
		return nil, errors.New("作品缺少 targets")
	}
	count := 0
	for _, t := range p.Targets {
		count += len(t.Blocks)
	}
	if count > MaxBlocks {
		return nil, fmt.Errorf("积木数量超过 %d", MaxBlocks)
	}
	return &p, nil
}

func decodeBlocks(t Target) (map[string]Block, int, error) {
	blocks := make(map[string]Block, len(t.Blocks))
	floating := 0
	for id, raw := range t.Blocks {
		if len(raw) > 0 && raw[0] == '[' {
			var a []json.RawMessage
			if json.Unmarshal(raw, &a) != nil || len(a) < 3 {
				return nil, 0, errors.New("变量积木格式无效")
			}
			if len(a) > 3 {
				floating++
			}
			continue
		}
		var b Block
		if json.Unmarshal(raw, &b) != nil || b.Opcode == "" {
			return nil, 0, errors.New("积木缺少 opcode")
		}
		blocks[id] = b
	}
	return blocks, floating, nil
}

// references returns actual block references and inline variable/list reporters.
func references(b Block) ([]string, int) {
	refs := []string{}
	variables := 0
	if b.Next != nil {
		refs = append(refs, *b.Next)
	}
	if b.Opcode == "procedures_definition" {
		return refs, variables
	}
	for _, raw := range b.Inputs {
		var input []json.RawMessage
		if json.Unmarshal(raw, &input) != nil || len(input) < 2 {
			continue
		}
		var kind int
		_ = json.Unmarshal(input[0], &kind)
		if kind == 12 || kind == 13 {
			variables++
			continue
		}
		if kind != 1 && kind != 2 && kind != 3 {
			continue
		}
		var ref string
		if json.Unmarshal(input[1], &ref) == nil && ref != "" {
			refs = append(refs, ref)
			continue
		}
		var literal []json.RawMessage
		if json.Unmarshal(input[1], &literal) == nil && len(literal) > 0 {
			var typ int
			_ = json.Unmarshal(literal[0], &typ)
			if typ == 12 || typ == 13 {
				variables++
			}
		}
	}
	return refs, variables
}
