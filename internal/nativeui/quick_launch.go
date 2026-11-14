package nativeui

/**
 * [INPUT]: 依赖 window.go 的工作区与最近列表、welcome.go 的 recentBranches（最近仓库的分支名）、mygo/ui 的 Modal/键盘事件
 * [OUTPUT]: 对外提供 Quick Launch（⇧⌘O）：quickItems（工作区与最近的仓库，按输入过滤、名字命中靠前，输入路径时多一行"打开该路径"）、quickRow（一行：文件夹、命中高亮的名字、路径与分支、当前工作区的对勾/有草稿的圆点；工作区下拉共用）、openQuickLaunch、quickLaunchView（浮层：输入框 + 两节列表 + 底部键位提示；↑↓ 选、↵ 切换或打开、esc 关）
 * [POS]: 打开仓库的快捷路径：同一个仓库根只有一个工作区，所以"打开"一个已在清单里的仓库就是切过去；列表项的"动作"全部走 window.go（chooseWorkspace / OpenRepository）
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
)

// quickLaunch is the palette's state: open, the query, the highlighted row.
type quickLaunch struct {
	open  bool
	tab   int // quickTabFiles / quickTabText / quickTabRepos
	query string
	sel   int
	qo    *quickOpenState // the Files / Text tabs' rows and preview (quick_open.go)
}

// quickItem is one row of Quick Launch.
type quickItem struct {
	Name, Path  string // Path is the repository root, absolute
	Branch      string // "" until known
	Open        bool   // a workspace
	WorkspaceID int    // the workspace, when Open
	Current     bool   // the workspace in front
	Dirty       bool   // a commit message or a Git operation is pending in it
	Typed       bool   // the "open the path you typed" row
}

// quickMatch scores how well a repository answers the query: every
// whitespace-separated token must occur in its name or path (case-insensitive).
// 0 = the name starts with the first token, 1 = the name contains all of them,
// 2 = only the path does; ok is false when a token occurs in neither.
func quickMatch(query, name, path string) (score int, ok bool) {
	tokens := strings.Fields(strings.ToLower(query))
	if len(tokens) == 0 {
		return 0, true
	}
	lname, lpath := strings.ToLower(name), strings.ToLower(path)
	inName := true
	for _, token := range tokens {
		switch {
		case strings.Contains(lname, token):
		case strings.Contains(lpath, token):
			inName = false
		default:
			return 0, false
		}
	}
	switch {
	case !inName:
		return 2, true
	case strings.HasPrefix(lname, tokens[0]):
		return 0, true
	}
	return 1, true
}

// quickItems is the list for a query: the typed path (when the query reads as
// one), then the workspaces, then the recent repositories that are not one —
// each section filtered by the query and ordered by how well the name matches,
// otherwise as they were.
func quickItems(open, recent []quickItem, query string) []quickItem {
	var out []quickItem
	if path := strings.TrimSpace(query); strings.HasPrefix(path, "/") || strings.HasPrefix(path, "~") {
		if abs := expandHome(path); filepath.IsAbs(abs) {
			out = append(out, quickItem{Name: filepath.Base(abs), Path: abs, Typed: true})
		}
	}
	filter := func(items []quickItem) []quickItem {
		type scored struct {
			item  quickItem
			score int
		}
		var hits []scored
		for _, item := range items {
			if score, ok := quickMatch(query, item.Name, item.Path); ok {
				hits = append(hits, scored{item, score})
			}
		}
		sort.SliceStable(hits, func(i, j int) bool { return hits[i].score < hits[j].score })
		result := make([]quickItem, len(hits))
		for i, h := range hits {
			result[i] = h.item
		}
		return result
	}
	out = append(out, filter(open)...)
	return append(out, filter(recent)...)
}

// quickRows gathers the palette's source rows from the window: every
// workspace, and the recent roots that are not one.
func (w *Window) quickRows() (open, recent []quickItem) {
	labels := w.workspaceLabels()
	shown := map[string]bool{}
	for i, ws := range w.workspaces {
		shown[ws.sh.dir] = true
		open = append(open, quickItem{
			Name: labels[i], Path: ws.sh.dir, Open: true, WorkspaceID: ws.id,
			Current: i == w.active, Dirty: w.workspaceNeedsConfirm(ws),
			Branch: ws.sh.currentBranchName(),
		})
	}
	for _, root := range w.settings.Window.Recent {
		if !shown[root] {
			recent = append(recent, quickItem{Name: pathBase(root), Path: root, Branch: w.recentBranch[root]})
		}
	}
	return open, recent
}

// openQuickLaunch shows the palette with an empty query.
func (w *Window) openQuickLaunch() { w.openQuickTab(quickTabRepos) }

// launch acts on a row: switch to a workspace, otherwise open the repository
// (which makes it one).
func (w *Window) launch(item quickItem) {
	w.quick.open = false
	if item.Open {
		w.chooseWorkspace(item.WorkspaceID)
		return
	}
	w.OpenRepository(item.Path, "", "")
}

// quickLaunchView is the palette: a query field over the workspaces and the
// recent repositories. ↑ ↓ move, ↵ switches or opens, esc closes.
func (w *Window) quickLaunchView(c *ui.Context) {
	q := &w.quick
	if !q.open {
		return
	}
	t := c.Theme()
	k := gorexColorsOf(t.Dark)
	width := float32(560)
	if q.tab != quickTabRepos {
		width = 900
	}
	ui.Modal(c, &q.open, func() {
		ui.Column(c).Width(width).MaxWidth(width + 60).Gap(Spacing().S).Children(func() {
			tab := q.tab
			segmentedControl(c, &q.tab, "Files", "Text", "Repos").Label("Quick open")
			if q.tab != tab { // the query stays: the same words, another place to look
				q.sel = 0
				return
			}
			if q.tab != quickTabRepos {
				w.quickOpenBody(c, q, k)
				return
			}
			w.quickReposBody(c, q, k)
		})
	})
}

// quickReposBody is the Repos tab, the former Quick Launch: the workspaces
// and the recent repositories under a query field.
func (w *Window) quickReposBody(c *ui.Context, q *quickLaunch, k *gorexColors) {
	t := c.Theme()
	open, recent := w.quickRows()
	items := quickItems(open, recent, q.query)
	q.sel = clamp(q.sel, 0, max(len(items)-1, 0))
	field := searchField(c, &q.query).FillWidth().AutoFocus().Label("Quick Launch").
		Placeholder("Open a repository, or type a folder path")
	if field.Changed() {
		q.sel = 0
	}
	if field.Shortcut(0, ui.KeyDown) {
		q.sel = min(q.sel+1, max(len(items)-1, 0))
	}
	if field.Shortcut(0, ui.KeyUp) {
		q.sel = max(q.sel-1, 0)
	}
	if field.Submitted() && len(items) > 0 {
		w.launch(items[q.sel])
		return
	}
	ui.Scroll(c).FillWidth().MaxHeight(switcherListMaxHeight + 60).Children(func() {
		ui.Column(c).FillWidth().Gap(1).Children(func() {
			section := ""
			for i, item := range items {
				head := "Recent"
				switch {
				case item.Typed:
					head = "Path"
				case item.Open:
					head = "Workspaces"
				}
				if head != section {
					section = head
					ui.Text(c, head).FontSize(Typography().BodySmall).FontWeight(WeightMedium).
						TextColor(t.TextMuted).Padding(6, 10, 2)
				}
				if w.quickRow(c, k, item, q.query, i == q.sel, "quick").Clicked() {
					w.launch(item)
				}
			}
			if len(items) == 0 {
				ui.Text(c, "No matching repositories").FontSize(Typography().Caption).
					TextColor(t.TextMuted).Padding(Spacing().S)
			}
		})
	})
	ui.Row(c).FillWidth().Padding(4, 10, 0).Gap(12).Children(func() {
		ui.Text(c, "↵ switch or open").FontSize(Typography().Caption).TextColor(t.TextMuted)
		ui.Text(c, "esc close").FontSize(Typography().Caption).TextColor(t.TextMuted)
		ui.Spacer(c)
		if compactButton(c, "Open Repository…").Clicked() {
			q.open = false
			w.chooseRepository()
		}
	})
}

// quickRow is one row: the folder glyph, the name with the matched part in
// the accent, the path and branch, and — for a workspace — a dot while
// something in it is pending and a check on the one in front. scope keeps the
// keys of two lists that show the same repository apart.
func (w *Window) quickRow(c *ui.Context, k *gorexColors, item quickItem, query string, selected bool, scope string) ui.Element {
	t := c.Theme()
	tokens := designTokens(t.Dark)
	var row ui.Element
	ui.Box(c).Children(func() {
		row = ui.ButtonBase(c.Key(scope+":"+item.Path)).FillWidth().Height(32).Radius(Radius().Row).
			Padding(0, 10).Gap(8).AlignItems(ui.Center).Label(item.Name)
		if tint, ok := rowTint(tokens, selected, row.Hovered()); ok {
			row.Background(tint)
		}
		row.Children(func() {
			ui.Icon(c, iconFolder).Size(Controls().IconRegular, Controls().IconRegular).TextColor(t.TextMuted).Shrink(0).PassThrough()
			pre, mid, post := splitMatch(item.Name, query)
			ui.Row(c).Shrink(0).AlignItems(ui.Center).PassThrough().Children(func() {
				if pre != "" {
					ui.Text(c, pre).FontSize(Typography().Section).SingleLine().PassThrough()
				}
				if mid != "" {
					ui.Text(c, mid).FontSize(Typography().Section).FontWeight(WeightSemibold).TextColor(t.Accent).
						SingleLine().PassThrough()
				}
				if post != "" {
					ui.Text(c, post).FontSize(Typography().Section).SingleLine().PassThrough()
				}
			})
			detail := tildePath(item.Path)
			if item.Branch != "" {
				detail += " · " + item.Branch
			}
			ui.Text(c, detail).FontSize(Typography().BodySmall).TextColor(t.TextMuted).SingleLine().
				Ellipsis("…").Grow(1).MinWidth(0).PassThrough()
			if item.Dirty {
				ui.Box(c).Size(6, 6).Radius(Radius().Pill).Shrink(0).PassThrough().
					Background(tokens.StatusColor(ToneWarning, t.Dark))
			}
			if item.Current {
				ui.Icon(c, iconCheck).Size(Controls().IconRegular, Controls().IconRegular).TextColor(t.Accent).Shrink(0).PassThrough()
			}
		})
	})
	return row
}

// splitMatch cuts name around the first occurrence of the query's first token
// (case-insensitive): what comes before, the match, what comes after. No
// match, or an empty query, leaves the whole name before.
func splitMatch(name, query string) (pre, mid, post string) {
	tokens := strings.Fields(query)
	if len(tokens) == 0 {
		return name, "", ""
	}
	lower := strings.ToLower(name)
	at := strings.Index(lower, strings.ToLower(tokens[0]))
	if at < 0 || len(lower) != len(name) { // a case fold that changes length cannot be sliced by byte
		at = -1
	}
	if at < 0 {
		return name, "", ""
	}
	end := at + len(tokens[0])
	return name[:at], name[at:end], name[end:]
}

// isDir reports whether path names an existing directory.
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
