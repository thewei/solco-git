package lsp

/**
 * [INPUT]: 语言服务器的 JSON（hover / definition / publishDiagnostics 的结果）
 * [OUTPUT]: 本包自己的小类型（Position、Range、Location、Diagnostic）、UTF-16 列 ⇄ 字节列、file:// URI ⇄ 路径、hover 内容转纯文本
 * [POS]: 只含只读能力要的几个结构；位置一律是 LSP 默认的 UTF-16，字节与它的换算只在这里
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"encoding/json"
	"errors"
	"net/url"
	"path/filepath"
	"sort"
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
	return strings.TrimSuffix(li.text[li.starts[i]:end], "\r") // a CRLF file's columns stop before the \r
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

// TextEdit is a change a server asks for: replace Range with NewText.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// ByteEdit is a TextEdit in bytes of one text (the offsets are into the text
// as it was before any edit is applied).
type ByteEdit struct {
	Start, End int
	Text       string
}

// ByteEdits converts server edits to byte edits of text, in order of position.
func ByteEdits(text string, edits []TextEdit) []ByteEdit {
	li := newLineIndex(text)
	out := make([]ByteEdit, 0, len(edits))
	for _, e := range edits {
		start, end := li.offset(e.Range.Start), li.offset(e.Range.End)
		out = append(out, ByteEdit{Start: start, End: max(start, end), Text: e.NewText})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}

// ApplyEdits applies server edits to text and returns the result. Overlapping
// edits are cut to follow the one before, as the editor's own Edit does.
func ApplyEdits(text string, edits []TextEdit) string {
	var b strings.Builder
	at := 0
	for _, e := range ByteEdits(text, edits) {
		start := max(e.Start, at)
		b.WriteString(text[at:start])
		b.WriteString(e.Text)
		at = max(at, e.End)
	}
	b.WriteString(text[at:])
	return b.String()
}

// FileEdit is the edits of one file of a workspace edit.
type FileEdit struct {
	Path  string
	Edits []TextEdit
}

// workspaceEdit reads a rename answer: `changes` (what the client asked for
// by not declaring documentChanges) or, from a server that sends it anyway,
// `documentChanges` holding only text edits. A file create / rename / delete
// is refused: the app applies text edits, nothing else.
func workspaceEdit(raw json.RawMessage) ([]FileEdit, error) {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var we struct {
		Changes         map[string][]TextEdit `json:"changes"`
		DocumentChanges []json.RawMessage     `json:"documentChanges"`
	}
	if err := json.Unmarshal(raw, &we); err != nil {
		return nil, err
	}
	byPath := map[string][]TextEdit{}
	var order []string
	add := func(uri string, edits []TextEdit) error {
		path, ok := URIPath(uri)
		if !ok {
			return errors.New("the rename reaches something that is not a file")
		}
		if _, seen := byPath[path]; !seen {
			order = append(order, path)
		}
		byPath[path] = append(byPath[path], edits...)
		return nil
	}
	for uri, edits := range we.Changes {
		if err := add(uri, edits); err != nil {
			return nil, err
		}
	}
	for _, dc := range we.DocumentChanges {
		var d struct {
			Kind         string `json:"kind"`
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
			Edits []TextEdit `json:"edits"`
		}
		if err := json.Unmarshal(dc, &d); err != nil {
			return nil, err
		}
		if d.Kind != "" {
			return nil, errors.New("the rename wants to create, rename or delete files, which Solco Git does not do")
		}
		if err := add(d.TextDocument.URI, d.Edits); err != nil {
			return nil, err
		}
	}
	sort.Strings(order)
	out := make([]FileEdit, 0, len(order))
	for _, p := range order {
		if len(byPath[p]) > 0 {
			out = append(out, FileEdit{Path: p, Edits: byPath[p]})
		}
	}
	return out, nil
}

// CompletionItem is a completion in the editor's terms: byte offsets of the
// text it was asked for.
type CompletionItem struct {
	Label, Detail, Doc string
	Kind               int // the protocol's CompletionItemKind
	Insert             string
	Snippet            bool // Insert is a snippet template
	Filter             string
	Start              int  // where the text it replaces begins; -1: the word at the caret
	NeedsMore          bool // it also edits elsewhere (an import): the editor cannot do that
}

// rawCompletionItem is a completion as it arrives.
type rawCompletionItem struct {
	Label               string            `json:"label"`
	Kind                int               `json:"kind"`
	Detail              string            `json:"detail"`
	Documentation       json.RawMessage   `json:"documentation"`
	SortText            string            `json:"sortText"`
	FilterText          string            `json:"filterText"`
	InsertText          string            `json:"insertText"`
	InsertTextFormat    int               `json:"insertTextFormat"`
	TextEdit            json.RawMessage   `json:"textEdit"`
	AdditionalTextEdits []json.RawMessage `json:"additionalTextEdits"`
}

// completionItems reads a completion answer — a list, or {isIncomplete,
// items} — into items sorted by the server's sortText, at most max.
func completionItems(raw json.RawMessage, idx *lineIndex, max int) []CompletionItem {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var items []rawCompletionItem
	if raw[0] == '[' {
		if json.Unmarshal(raw, &items) != nil {
			return nil
		}
	} else {
		var list struct {
			Items []rawCompletionItem `json:"items"`
		}
		if json.Unmarshal(raw, &list) != nil {
			return nil
		}
		items = list.Items
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i].SortText, items[j].SortText
		if a == "" {
			a = items[i].Label
		}
		if b == "" {
			b = items[j].Label
		}
		return a < b
	})
	var out []CompletionItem
	for _, it := range items {
		if it.Label == "" {
			continue
		}
		ci := CompletionItem{Label: it.Label, Detail: it.Detail, Doc: hoverText(it.Documentation), Kind: it.Kind,
			Filter: it.FilterText, Start: -1, Insert: it.InsertText, Snippet: it.InsertTextFormat == 2,
			NeedsMore: len(it.AdditionalTextEdits) > 0}
		if ci.Insert == "" {
			ci.Insert = it.Label
		}
		if len(it.TextEdit) > 0 && string(it.TextEdit) != "null" {
			var te struct {
				NewText string `json:"newText"`
				Range   *Range `json:"range"`
				Insert  *Range `json:"insert"`
			}
			if json.Unmarshal(it.TextEdit, &te) == nil {
				rng := te.Range
				if rng == nil {
					rng = te.Insert
				}
				if rng != nil {
					ci.Start = idx.offset(rng.Start)
				}
				ci.Insert = te.NewText
			}
		}
		out = append(out, ci)
		if len(out) >= max {
			break
		}
	}
	return out
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
