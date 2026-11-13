package nativeui

/**
 * [INPUT]: quick_open.go 的行与查询、filesview.MatchFiles 的匹配规则（逐词：名字前缀 / 名字子串 / 路径子串 / 名字子序列 / 路径子序列）
 * [OUTPUT]: 快开结果行里的匹配高亮（2026-11-13 决定第 (3) 条补）：Files 标签标出文件名与目录里命中的字符，Text 标签标出命中行里的查询；颜色与 Markdown 的查找标记同一个（markdown.Theme.Mark）
 * [POS]: 匹配规则在 solco-gitkit 里，这里只重放"命中了哪里"，不改变排序；大小写折叠改变字节数的路径 / 行不标（宁可不标也不标错位置）
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

// quickMatchRanges is where in path each word of query matched: the same
// rule MatchFiles ranks by, so what is marked is what made the row a hit.
func quickMatchRanges(path, query string) [][2]int {
	lower := strings.ToLower(path)
	if len(lower) != len(path) {
		return nil
	}
	cut := strings.LastIndexByte(lower, '/') + 1
	base := lower[cut:]
	var out [][2]int
	for _, w := range strings.Fields(strings.ToLower(query)) {
		switch {
		case strings.HasPrefix(base, w):
			out = append(out, [2]int{cut, cut + len(w)})
		case strings.Contains(base, w):
			at := cut + strings.Index(base, w)
			out = append(out, [2]int{at, at + len(w)})
		case strings.Contains(lower, w):
			at := strings.Index(lower, w)
			out = append(out, [2]int{at, at + len(w)})
		default:
			if r := subsequenceRanges(w, base, cut); r != nil {
				out = append(out, r...)
			} else if r := subsequenceRanges(w, lower, 0); r != nil {
				out = append(out, r...)
			}
		}
	}
	return mergeRanges(out)
}

// subsequenceRanges places word's runes, in order, in s (offset by off in the
// whole path); nil when they do not all occur.
func subsequenceRanges(word, s string, off int) [][2]int {
	var out [][2]int
	i := 0
	for pos, r := range s {
		if i >= len(word) {
			break
		}
		w, size := utf8.DecodeRuneInString(word[i:])
		if r == w {
			out = append(out, [2]int{off + pos, off + pos + utf8.RuneLen(r)})
			i += size
		}
	}
	if i != len(word) {
		return nil
	}
	return out
}

// quickTextRanges is every case-insensitive occurrence of query in a text
// hit's line.
func quickTextRanges(text, query string) [][2]int {
	q := strings.ToLower(strings.TrimSpace(query))
	lower := strings.ToLower(text)
	if q == "" || len(lower) != len(text) {
		return nil
	}
	var out [][2]int
	for pos := 0; ; {
		j := strings.Index(lower[pos:], q)
		if j < 0 {
			return out
		}
		out = append(out, [2]int{pos + j, pos + j + len(q)})
		pos += j + len(q)
	}
}

// mergeRanges sorts ranges and joins the ones that touch or overlap.
func mergeRanges(rs [][2]int) [][2]int {
	if len(rs) < 2 {
		return rs
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i][0] < rs[j][0] })
	out := [][2]int{rs[0]}
	for _, r := range rs[1:] {
		last := &out[len(out)-1]
		if r[0] <= last[1] {
			last[1] = max(last[1], r[1])
		} else {
			out = append(out, r)
		}
	}
	return out
}

// markedSpans cuts text[from:to] (of a longer string the ranges index) at the
// ranges: the marked stretches get mark's background and weight.
func markedSpans(text string, from, to int, ranges [][2]int, mark ui.Color) []ui.Span {
	var out []ui.Span
	at := from
	for _, r := range ranges {
		a, b := max(r[0], from), min(r[1], to)
		if a >= b {
			continue
		}
		if a > at {
			out = append(out, ui.Span{Text: text[at:a]})
		}
		out = append(out, ui.Span{Text: text[a:b], Background: mark, Weight: 600})
		at = b
	}
	if at < to {
		out = append(out, ui.Span{Text: text[at:to]})
	}
	return out
}
