package lsp

/**
 * [INPUT]: 语言服务器的 JSON（hover / definition / publishDiagnostics 的结果）
 * [OUTPUT]: 本包自己的小类型（Position、Range、Location、Diagnostic）、UTF-16 列 ⇄ 字节列、file:// URI ⇄ 路径、hover 内容转纯文本
 * [POS]: 只含只读能力要的几个结构；位置一律是 LSP 默认的 UTF-16，字节与它的换算只在这里
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

// Position is a line and a column in UTF-16 code units, both from 0.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range is Start up to End.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location is a range of a file on disk.
type Location struct {
	Path  string
	Range Range
}

// Severity values as the protocol numbers them.
const (
	SevError   = 1
	SevWarning = 2
	SevInfo    = 3
	SevHint    = 4
)

// Diagnostic is a problem a server reported, with byte offsets into the
// text of the version it was made for.
type Diagnostic struct {
	Start, End int
	Severity   int
	Message    string
	Source     string
}

// UTF16Col is the UTF-16 column of byteCol (a byte offset into line).
func UTF16Col(line string, byteCol int) int {
	byteCol = max(0, min(byteCol, len(line)))
	n := 0
	for _, r := range line[:byteCol] {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// ByteCol is the byte offset in line of the UTF-16 column col, clamped to
// the line. A column inside a surrogate pair lands after the character.
func ByteCol(line string, col int) int {
	if col <= 0 {
		return 0
	}
	n := 0
	for i, r := range line {
		if n >= col {
			return i
		}
		n += len(utf16.Encode([]rune{r}))
	}
	return len(line)
}

// FileURI is the file:// URI of an absolute path.
func FileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

// URIPath is the path of a file:// URI; ok is false for any other scheme.
func URIPath(uri string) (string, bool) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return "", false
	}
	return filepath.FromSlash(u.Path), true
}

// lineIndex is the byte offset each line of text starts at.
type lineIndex struct {
	text   string
	starts []int
}

func newLineIndex(text string) *lineIndex {
	li := &lineIndex{text: text, starts: []int{0}}
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			li.starts = append(li.starts, i+1)
		}
	}
	return li
}

func (li *lineIndex) line(i int) string {
	if i < 0 || i >= len(li.starts) {
		return ""
	}
	end := len(li.text)
	if i+1 < len(li.starts) {
		end = li.starts[i+1] - 1
	}
	return li.text[li.starts[i]:end]
}

// offset is the byte offset of a protocol position, clamped into the text.
func (li *lineIndex) offset(p Position) int {
	if p.Line < 0 {
		return 0
	}
	if p.Line >= len(li.starts) {
		return len(li.text)
	}
	return li.starts[p.Line] + ByteCol(li.line(p.Line), p.Character)
}

// position is the protocol position of a byte offset.
func (li *lineIndex) position(off int) Position {
	off = max(0, min(off, len(li.text)))
	lo, hi := 0, len(li.starts)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if li.starts[mid] <= off {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return Position{Line: lo, Character: UTF16Col(li.line(lo), off-li.starts[lo])}
}

// rawDiagnostic is a diagnostic as it arrives.
type rawDiagnostic struct {
	Range    Range           `json:"range"`
	Severity int             `json:"severity"`
	Message  string          `json:"message"`
	Source   string          `json:"source"`
	Code     json.RawMessage `json:"code"`
}

// hoverText is the plain text of a hover's contents, which is a string, a
// {language,value} pair, a {kind,value} markup, or an array of those. Code
// fences are dropped: the editor shows the text in its own font.
func hoverText(raw json.RawMessage) string {
	var parts []string
	var walk func(raw json.RawMessage)
	walk = func(raw json.RawMessage) {
		raw = json.RawMessage(strings.TrimSpace(string(raw)))
		if len(raw) == 0 || string(raw) == "null" {
			return
		}
		switch raw[0] {
		case '"':
			var s string
			if json.Unmarshal(raw, &s) == nil {
				parts = append(parts, s)
			}
		case '[':
			var arr []json.RawMessage
			if json.Unmarshal(raw, &arr) == nil {
				for _, a := range arr {
					walk(a)
				}
			}
		case '{':
			var o struct {
				Value string `json:"value"`
			}
			if json.Unmarshal(raw, &o) == nil && o.Value != "" {
				parts = append(parts, o.Value)
			}
		}
	}
	walk(raw)
	var lines []string
	for _, p := range parts {
		for _, ln := range strings.Split(strings.ReplaceAll(p, "\r\n", "\n"), "\n") {
			if strings.HasPrefix(strings.TrimSpace(ln), "```") {
				continue
			}
			lines = append(lines, ln)
		}
		lines = append(lines, "")
	}
	text := strings.TrimSpace(strings.Join(lines, "\n"))
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}
	return text
}

// locations reads a definition answer: null, a Location, a Location[], or a
// LocationLink[] (whose selection range is what is shown).
func locations(raw json.RawMessage) []Location {
	type loc struct {
		URI                  string `json:"uri"`
		Range                Range  `json:"range"`
		TargetURI            string `json:"targetUri"`
		TargetSelectionRange *Range `json:"targetSelectionRange"`
		TargetRange          Range  `json:"targetRange"`
	}
	var list []loc
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	switch {
	case len(raw) == 0 || string(raw) == "null":
		return nil
	case raw[0] == '[':
		if json.Unmarshal(raw, &list) != nil {
			return nil
		}
	default:
		var one loc
		if json.Unmarshal(raw, &one) != nil {
			return nil
		}
		list = []loc{one}
	}
	var out []Location
	for _, l := range list {
		uri, rng := l.URI, l.Range
		if l.TargetURI != "" {
			uri, rng = l.TargetURI, l.TargetRange
			if l.TargetSelectionRange != nil {
				rng = *l.TargetSelectionRange
			}
		}
		if p, ok := URIPath(uri); ok {
			out = append(out, Location{Path: p, Range: rng})
		}
	}
	return out
}
