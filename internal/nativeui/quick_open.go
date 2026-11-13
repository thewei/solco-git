package nativeui

/**
 * [INPUT]: quick_launch.go 的浮层与 quickLaunch 状态、files_search.go 的文件索引（filesSearchEnsureIndex）、filesview.MatchFiles / SearchContent、editor.State
 * [OUTPUT]: 快开的 Files 与 Text 两个标签（2026-11-13 决定第 (3) 条，E2）：⌘P（Files：当前工作区的文件名，子序列匹配，空输入列最近打开的）、⇧⌘F（Text：内容搜索，停顿后才起进程）；↑↓ 选、↵ 打开（Text 落在命中行）、右侧只读预览（Files 看开头，Text 落在命中行）；Repos 标签就是原 Quick Launch（⇧⌘O）
 * [POS]: 不需要 fzf：匹配是 filesview 的 Go 实现；索引与侧栏搜索共用同一份（刷新 / 文件操作后一并失效）；预览读文件的前 256 KiB，不是 Files 视图的那一个编辑器
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/thewei/solco-gitkit/filesview"
)

// The palette's tabs, in the order of its segmented control.
const (
	quickTabFiles = iota
	quickTabText
	quickTabRepos
)

const (
	quickOpenRows   = 80
	quickPreviewCap = 256 << 10
)

// quickOpenRow is one result: a file, and for a text hit the line.
type quickOpenRow struct {
	rel               string   // slash-separated, relative to the root
	line, col, length int      // line from 1; 0 for a file row
	text              string   // the matching line, for a text hit
	hl                [][2]int // where the query matched in rel (a file row) or in text (a text hit)
}

// quickOpenState is the Files / Text tabs' results and preview. The rows
// belong to the workspace in front when the key was made.
type quickOpenState struct {
	key     string
	rows    []quickOpenRow
	total   int
	note    string
	loading bool

	gen    atomic.Uint64
	cancel context.CancelFunc

	// The preview of the selected row: what was asked for and what was read.
	prevAsk  string
	prev     *editor.State
	prevNote string
	prevGen  atomic.Uint64
}

// openQuickTab shows the palette on a tab with an empty query.
func (w *Window) openQuickTab(tab int) {
	if tab != quickTabRepos && w.activeShell() == nil {
		tab = quickTabRepos // no workspace to look into
	}
	if w.quick.qo != nil && w.quick.qo.cancel != nil {
		w.quick.qo.cancel()
	}
	w.quick = quickLaunch{open: true, tab: tab}
	w.loadRecentBranches()
}

// quickOpenSync keeps the rows in step with the query, the tab and the index.
func (w *Window) quickOpenSync(sh *Shell, q *quickLaunch) {
	if q.qo == nil {
		q.qo = &quickOpenState{}
	}
	qo := q.qo
	sh.syncRightPanelRoot()
	idx := &sh.filesSearch
	if q.tab == quickTabFiles {
		sh.filesSearchEnsureIndex()
	}
	query := strings.TrimSpace(q.query)
	key := strings.Join([]string{itoa(q.tab), query, sh.rightPanel.currentRoot, boolKey(idx.indexReady), idx.indexErr, boolKey(sh.rightPanel.showIgnored)}, "\x00")
	if key == qo.key {
		return
	}
	qo.key = key
	qo.gen.Add(1)
	if qo.cancel != nil {
		qo.cancel()
		qo.cancel = nil
	}
	qo.rows, qo.total, qo.note, qo.loading = nil, 0, "", false
	q.sel = 0
	root := sh.rightPanel.currentRoot
	switch q.tab {
	case quickTabFiles:
		switch {
		case query == "":
			for _, p := range sh.recentFiles {
				if rel, ok := sh.repoRel(p); ok {
					qo.rows = append(qo.rows, quickOpenRow{rel: filepath.ToSlash(rel)})
				}
			}
			qo.total = len(qo.rows)
		case idx.indexErr != "":
			qo.note = idx.indexErr
		case !idx.indexReady:
			qo.loading = true
		default:
			hits, total := filesview.MatchFiles(idx.index, query, quickOpenRows)
			for _, h := range hits {
				qo.rows = append(qo.rows, quickOpenRow{rel: h, hl: quickMatchRanges(h, query)})
			}
			qo.total = total
		}
	case quickTabText:
		if len([]rune(query)) < filesview.MinContentQuery {
			return
		}
		w.quickOpenRunText(sh, qo, root, query)
	}
}

func boolKey(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// quickOpenRunText searches file text after a short pause; the rows of the
// last query stay until the answer lands.
func (w *Window) quickOpenRunText(sh *Shell, qo *quickOpenState, root, query string) {
	gen := qo.gen.Load()
	ctx, cancel := context.WithTimeout(context.Background(), filesview.SearchDeadline)
	qo.cancel = cancel
	qo.loading = true
	cq := filesview.ContentQuery{Pattern: query, IncludeIgnored: sh.rightPanel.showIgnored}
	apply := func(res filesview.ContentResult, err error) {
		if gen != qo.gen.Load() {
			return
		}
		qo.loading = false
		if err != nil {
			qo.note = filesSearchError(err)
			return
		}
		for _, f := range res.Files {
			for _, m := range f.Matches {
				if len(qo.rows) >= quickOpenRows {
					break
				}
				length := m.End - m.Start
				if m.Start < 0 || length < 0 {
					length = 0
				}
				qo.rows = append(qo.rows, quickOpenRow{rel: f.Path, line: m.Line, col: m.Col, length: length, text: m.Text,
					hl: quickTextRanges(m.Text, cq.Pattern)})
			}
		}
		qo.total = res.Matches
	}
	if w.win == nil {
		res, err := filesview.SearchContent(ctx, root, cq)
		cancel()
		apply(res, err)
		return
	}
	go func() {
		defer cancel()
		select {
		case <-time.After(filesSearchDebounce):
		case <-ctx.Done():
			return
		}
		res, err := filesview.SearchContent(ctx, root, cq)
		w.onUI(func() { apply(res, err) })
	}()
}

// quickOpenAct opens a row in the Files view: the file, or the line a text
// hit points at.
func (w *Window) quickOpenAct(sh *Shell, row quickOpenRow) {
	w.quick.open = false
	abs := filepath.Join(sh.rightPanel.currentRoot, filepath.FromSlash(row.rel))
	if row.line > 0 {
		sh.revealFileAt(abs, row.line-1, row.col, row.length)
		return
	}
	sh.revealFile(abs)
}

// quickOpenPreview reads what the selected row shows, off the UI lane.
func (w *Window) quickOpenPreview(sh *Shell, qo *quickOpenState, row *quickOpenRow) {
	if row == nil {
		qo.prevAsk, qo.prev, qo.prevNote = "", nil, ""
		return
	}
	ask := row.rel + "\x00" + itoa(row.line)
	if ask == qo.prevAsk {
		return
	}
	qo.prevAsk, qo.prev, qo.prevNote = ask, nil, ""
	gen := qo.prevGen.Add(1)
	abs := filepath.Join(sh.rightPanel.currentRoot, filepath.FromSlash(row.rel))
	line := row.line - 1
	name := filepath.Base(abs)
	type loaded struct {
		buf  *editor.Buffer
		note string
	}
	load := func() loaded {
		f, err := os.Open(abs)
		if err != nil {
			return loaded{note: "Can't read this file"}
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, quickPreviewCap))
		if err != nil {
			return loaded{note: "Can't read this file"}
		}
		if filesview.IsBinary(data) {
			return loaded{note: "Binary file"}
		}
		b := editor.NewBuffer(strings.ToValidUTF8(string(data), "\uFFFD"))
		b.SetLanguage(editor.LanguageFor(name))
		return loaded{buf: b}
	}
	apply := func(l loaded) {
		if gen != qo.prevGen.Load() {
			return
		}
		qo.prevNote = l.note
		if l.buf == nil {
			return
		}
		st := &editor.State{Buffer: l.buf, ReadOnly: true, NoActiveLine: true, Label: name}
		if line > 0 {
			st.GoTo(editor.Pos{Line: min(line, max(l.buf.LineCount()-1, 0))})
		}
		qo.prev = st
	}
	if w.win == nil {
		apply(load())
		return
	}
	go func() {
		l := load()
		w.onUI(func() { apply(l) })
	}()
}

// quickOpenBody is the Files / Text tab: the query field, the rows and the
// preview of the selected one.
func (w *Window) quickOpenBody(c *ui.Context, q *quickLaunch, k *gorexColors) {
	t := c.Theme()
	sh := w.activeShell()
	if sh == nil {
		ui.Text(c, "Open a repository first").FontSize(Typography().Caption).TextColor(t.TextMuted).Padding(Spacing().S)
		return
	}
	placeholder := "Go to file"
	if q.tab == quickTabText {
		placeholder = "Find in files"
	}
	field := searchField(c, &q.query).FillWidth().AutoFocus().Label("Quick open").Placeholder(placeholder)
	w.quickOpenSync(sh, q)
	qo := q.qo
	if field.Changed() {
		q.sel = 0
	}
	n := len(qo.rows)
	if field.Shortcut(0, ui.KeyDown) {
		q.sel = min(q.sel+1, max(n-1, 0))
	}
	if field.Shortcut(0, ui.KeyUp) {
		q.sel = max(q.sel-1, 0)
	}
	q.sel = clamp(q.sel, 0, max(n-1, 0))
	if field.Submitted() && n > 0 {
		w.quickOpenAct(sh, qo.rows[q.sel])
		return
	}
	var selected *quickOpenRow
	if n > 0 {
		selected = &qo.rows[q.sel]
	}
	w.quickOpenPreview(sh, qo, selected)

	ui.Row(c).FillWidth().Gap(Spacing().M).Children(func() {
		ui.Scroll(c).Width(340).Shrink(0).Height(switcherListMaxHeight + 120).Children(func() {
			ui.Column(c).FillWidth().Gap(1).Children(func() {
				for i := range qo.rows {
					if w.quickOpenRowView(c, k, &qo.rows[i], i == q.sel, i).Clicked() {
						q.sel = i
						w.quickOpenAct(sh, qo.rows[i])
						return
					}
				}
				msg := ""
				switch {
				case qo.note != "":
					msg = qo.note
				case qo.loading:
					msg = "Searching…"
				case n == 0 && strings.TrimSpace(q.query) == "" && q.tab == quickTabFiles:
					msg = "No files opened yet — type to search"
				case n == 0 && q.tab == quickTabText && len([]rune(strings.TrimSpace(q.query))) < filesview.MinContentQuery:
					msg = "Type at least two characters"
				case n == 0:
					msg = "No matches"
				}
				if msg != "" {
					ui.Text(c, msg).FontSize(Typography().Caption).TextColor(t.TextMuted).Padding(Spacing().S)
				}
			})
		})
		ui.Box(c).Grow(1).MinWidth(0).Height(switcherListMaxHeight + 120).Radius(Radius().Row).
			BorderWidth(1).BorderColor(designTokens(t.Dark).BorderSubtle).Children(func() {
			switch {
			case qo.prev != nil:
				qo.prev.Theme = codeEditorTheme(t)
				editor.View(c, qo.prev).FillWidth().Grow(1).MinHeight(0)
			case qo.prevNote != "":
				ui.Text(c, qo.prevNote).FontSize(Typography().Caption).TextColor(t.TextMuted).Padding(Spacing().M)
			}
		})
	})
	status := ""
	if qo.total > n {
		status = "Showing " + itoa(n) + " of " + itoa(qo.total)
	}
	ui.Row(c).FillWidth().Padding(4, 10, 0).Gap(12).Children(func() {
		ui.Text(c, "↵ open").FontSize(Typography().Caption).TextColor(t.TextMuted)
		ui.Text(c, "↑↓ move").FontSize(Typography().Caption).TextColor(t.TextMuted)
		ui.Text(c, "esc close").FontSize(Typography().Caption).TextColor(t.TextMuted)
		ui.Spacer(c)
		ui.Text(c, status).FontSize(Typography().Caption).TextColor(t.TextMuted)
	})
}

// quickOpenRowView is one result: the file glyph, the name, the folder and,
// for a text hit, the line number and the matching line.
func (w *Window) quickOpenRowView(c *ui.Context, k *gorexColors, row *quickOpenRow, selected bool, i int) ui.Element {
	t := c.Theme()
	tokens := designTokens(t.Dark)
	mark := markdown.DefaultTheme(t).Mark
	name := filepath.Base(row.rel)
	dir := ""
	if d := filepath.Dir(row.rel); d != "." {
		dir = d
	}
	var el ui.Element
	ui.Box(c).Children(func() {
		el = ui.ButtonBase(c.Key("qo:"+itoa(i))).FillWidth().MinHeight(32).Radius(Radius().Row).
			Padding(4, 10).Gap(8).AlignItems(ui.Center).Label(name)
		if tint, ok := rowTint(tokens, selected, el.Hovered()); ok {
			el.Background(tint)
		}
		el.Children(func() {
			ui.Icon(c, iconFile).Size(15, 15).TextColor(t.TextMuted).Shrink(0).PassThrough()
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).PassThrough().Children(func() {
					if cut := len(row.rel) - len(name); row.line == 0 && len(row.hl) > 0 {
						ui.RichText(c, markedSpans(row.rel, cut, len(row.rel), row.hl, mark)...).
							FontSize(Typography().Section).SingleLine().Shrink(0).PassThrough()
					} else {
						ui.Text(c, name).FontSize(Typography().Section).SingleLine().Shrink(0).PassThrough()
					}
					if row.line > 0 {
						ui.Text(c, ":"+itoa(row.line)).FontSize(Typography().BodySmall).TextColor(t.TextMuted).SingleLine().Shrink(0).PassThrough()
					}
					if row.line == 0 && len(row.hl) > 0 && dir != "" {
						ui.RichText(c, markedSpans(row.rel, 0, len(dir), row.hl, mark)...).FontSize(Typography().BodySmall).
							TextColor(t.TextMuted).SingleLine().Ellipsis("…").Grow(1).MinWidth(0).PassThrough()
					} else {
						ui.Text(c, dir).FontSize(Typography().BodySmall).TextColor(t.TextMuted).SingleLine().
							Ellipsis("…").Grow(1).MinWidth(0).PassThrough()
					}
				})
				if row.text != "" {
					if len(row.hl) > 0 {
						ui.RichText(c, markedSpans(row.text, 0, len(row.text), row.hl, mark)...).Font(gdCodeFont()).
							FontSize(Typography().BodySmall).TextColor(t.TextMuted).SingleLine().Ellipsis("…").MinWidth(0).PassThrough()
					} else {
						ui.Text(c, row.text).Font(gdCodeFont()).FontSize(Typography().BodySmall).TextColor(t.TextMuted).
							SingleLine().Ellipsis("…").MinWidth(0).PassThrough()
					}
				}
			})
		})
	})
	return el
}
