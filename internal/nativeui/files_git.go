package nativeui

/**
 * [INPUT]: 依赖 Shell 的 Git 快照（gitSnapshot）与 Runner（Blame/ReadBlob）、right_panel_files.go 的树行与导航模型、files_native_preview.go 的预览状态、MyGo ui/editor 的 DiffBase
 * [OUTPUT]: 对外提供 filesGitState、filesMarks/filesChangeMarks（改动文件→状态标记与含改动的目录）、filesScopeTabs（Changed n | All）、renderChangedTree（只看改动的树）、filesMark/filesDirDot（树行右侧的 M/U 小块与目录圆点）、readDiffBase（工作区文件对 HEAD 的槽边改动条）、filesGitControls（Plain|Blame 与 History）、filesBlameBody（逐行 blame）、filesPreviewFoot（图例与语言·编码·行数）
 * [POS]: Solco Git 在 Files 视图上比 Solco 多出来的那一层：全部读自 git 快照与 git blame/cat-file，不写任何东西；标记随快照走，不另起文件监听
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"bytes"
	"context"
	"path/filepath"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/thewei/solco-gitkit/filesview"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

// filesGitState is what the Files view keeps beyond the tree itself.
type filesGitState struct {
	changedOnly bool
	changedShut map[string]bool // folders folded in the Changed view, by absolute path
	marks       *filesMarks

	blame        bool
	blameFor     string
	blameLines   []gitworkbench.BlameLine
	blameLoading bool
	blameErr     string
	blameList    ui.ListState
	blameGen     int
}

// changedNode is one row of the Changed tree: a folder with its changed
// files below it, or a changed file.
type changedNode struct {
	name string
	path string // absolute
	dir  bool
	file *gitworkbench.ChangeFile
	kids []*changedNode
}

// filesMarks is the Git status laid over the Files tree: which files changed
// (and how), which folders hold a change, and the Changed view's own tree.
type filesMarks struct {
	snap  *gitworkbench.ChangesSnapshot
	root  string
	files map[string]*gitworkbench.ChangeFile
	dirs  map[string]bool
	tree  []*changedNode
	count int
}

// buildFilesMarks folds a snapshot's files into marks and the Changed tree:
// folders first, then files, each by name without regard to case.
func buildFilesMarks(snap *gitworkbench.ChangesSnapshot, root string) *filesMarks {
	m := &filesMarks{snap: snap, root: root, files: map[string]*gitworkbench.ChangeFile{}, dirs: map[string]bool{}}
	if snap == nil || root == "" {
		return m
	}
	nodes := map[string]*changedNode{}
	var top []*changedNode
	for i := range snap.Files {
		cf := &snap.Files[i]
		rel := strings.Trim(cf.Path, "/")
		if rel == "" {
			continue
		}
		m.count++
		parts := strings.Split(rel, "/")
		var parent *changedNode
		at := root
		for j, part := range parts {
			at = filepath.Join(at, part)
			if j == len(parts)-1 {
				m.files[at] = cf
				node := &changedNode{name: part, path: at, file: cf}
				if parent == nil {
					top = append(top, node)
				} else {
					parent.kids = append(parent.kids, node)
				}
				break
			}
			m.dirs[at] = true
			node := nodes[at]
			if node == nil {
				node = &changedNode{name: part, path: at, dir: true}
				nodes[at] = node
				if parent == nil {
					top = append(top, node)
				} else {
					parent.kids = append(parent.kids, node)
				}
			}
			parent = node
		}
	}
	sortChanged(top)
	m.tree = top
	return m
}

func sortChanged(nodes []*changedNode) {
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].dir != nodes[j].dir {
			return nodes[i].dir
		}
		return strings.ToLower(nodes[i].name) < strings.ToLower(nodes[j].name)
	})
	for _, n := range nodes {
		sortChanged(n.kids)
	}
}

// filesChangeMarks is the marks of the current snapshot, rebuilt only when
// the snapshot (or the folder the tree shows) changed.
func (s *Shell) filesChangeMarks() *filesMarks {
	root := s.rightPanel.currentRoot
	snap := s.gitSnapshot()
	g := &s.filesGit
	if g.marks == nil || g.marks.snap != snap || g.marks.root != root {
		g.marks = buildFilesMarks(snap, root)
	}
	return g.marks
}

// repoRel is path relative to the repository root, and whether it is inside.
func (s *Shell) repoRel(path string) (string, bool) {
	root := s.rightPanel.currentRoot
	if root == "" {
		return "", false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// filesScopeTabs is the tree's "Changed n | All" switch; it sits on the
// sidebar's material, so it wears the sidebar's segmented face.
func (s *Shell) filesScopeTabs(c *ui.Context, count int) {
	t := c.Theme()
	g := &s.filesGit
	chosen := 1
	if g.changedOnly {
		chosen = 0
	}
	tabs := ui.SegmentedBase(c, &chosen, 2)
	materialSegmentedTrack(c, tabs, "Show")
	tabs.Track.Children(func() {
		for i, label := range []string{"Changed", "All"} {
			seg, ink := materialSegmentFacePadded(c, tabs, i, i == chosen, label, t.Space(2))
			seg.Children(func() {
				ui.Text(c, label).FontSize(Typography().Body).FontWeight(WeightMedium).TextColor(ink).SingleLine()
				if i == 0 && count > 0 {
					ui.Text(c, itoa(count)).FontSize(Typography().Caption).FontWeight(WeightSemibold).TextColor(ink).SingleLine()
				}
			})
		}
	})
	if tabs.Track.Changed() {
		g.changedOnly = chosen == 0
	}
}

// renderChangedTree draws the Changed view: only the changed files, their
// folders open.
func (s *Shell) renderChangedTree(c *ui.Context, nodes []*changedNode, level int) {
	if level == 0 && len(nodes) == 0 {
		filesMutedRow(c, "No changed files", 0)
		return
	}
	g := &s.filesGit
	for _, node := range nodes {
		n := node
		if n.dir {
			open := !g.changedShut[n.path]
			_, clicked := s.sidebarTreeRow(c, sidebarTreeSpec{
				Key: "changed:" + n.path, Title: n.name, Label: "Folder " + n.name, Tooltip: n.path, Level: level,
				Folder: true, Expanded: open, Glyph: iconFolder, Nav: &s.nav.files, Motion: true,
				Trailing: func() { filesDirDot(c) },
			})
			if clicked {
				if g.changedShut == nil {
					g.changedShut = map[string]bool{}
				}
				g.changedShut[n.path] = open
			}
			if open {
				s.renderChangedTree(c, n.kids, level+1)
			}
			continue
		}
		cf := n.file
		gone := cf.Status == gitworkbench.StatusDeleted
		rel, _ := s.repoRel(n.path)
		spec := sidebarTreeSpec{
			Key: "changed:" + n.path, Title: n.name, Label: "File " + n.name, Tooltip: rel, Level: level,
			Glyph: iconFile, Dim: gone, Nav: &s.nav.files, Motion: true,
			Current: s.rightPanel.selectedFilePath == n.path, Selected: s.rightPanel.selectedFilePath == n.path,
			Trailing: func() { filesMark(c, cf) },
		}
		if !gone {
			spec.Menu = func(m *ui.Menu) {
				s.filesRowMenu(m, filesview.Entry{Name: n.name, Path: n.path, RelPath: rel})
			}
		}
		if _, clicked := s.sidebarTreeRow(c, spec); clicked {
			if gone {
				s.pendingToast = "A deleted file has nothing to open; its change is in Changes."
			} else {
				s.openFilePreview(n.path)
			}
		}
	}
}

// filesMark is a changed file's status tile, the same small tinted letter the
// Changes list wears.
func filesMark(c *ui.Context, cf *gitworkbench.ChangeFile) {
	t := c.Theme()
	letter := gdStatusLetterColor(cf.Status, gdPaletteFor(t), t)
	ui.Box(c).Size(16, 16).Shrink(0).Radius(treeRowRadius).Background(letter.Alpha(0.16)).Center().Children(func() {
		ui.Text(c, gdStatusLetter(cf)).Font(gdCodeFont()).FontSize(Typography().BodySmall).FontWeight(WeightSemibold).
			TextColor(letter).PassThrough()
	})
}

// filesDirDot marks a folder that holds a change.
func filesDirDot(c *ui.Context) {
	ui.Box(c).Size(6, 6).Shrink(0).Radius(3).Background(codeEditorTheme(c.Theme()).Modified).PassThrough()
}

// readDiffBase is the text a file's gutter is drawn against: its version in
// HEAD, or nothing at all for a file Git does not know yet (every line is
// then an addition). A binary or oversized blob, or a file outside Git's
// reach, draws no gutter. It runs off the UI lane and touches no UI state.
func readDiffBase(runner *gitworkbench.Runner, root, rel string, isNew bool) *editor.Buffer {
	if runner == nil || root == "" || rel == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitworkbench.ReadTimeout)
	defer cancel()
	data, err := runner.ReadBlob(ctx, root, "HEAD:"+rel, filesview.TextPreviewCap)
	if err != nil {
		if isNew {
			return editor.NewBuffer("")
		}
		return nil
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return nil
	}
	return editor.NewBuffer(string(data))
}

// diffBasePlan is what beginNativeText needs from the UI lane to ask for the
// gutter's base later: the repository-relative path and whether Git has never
// seen the file.
func (s *Shell) diffBasePlan(path string) (rel string, isNew, ok bool) {
	rel, ok = s.repoRel(path)
	if !ok || s.git == nil || s.git.root != s.rightPanel.currentRoot {
		return "", false, false
	}
	if cf := s.filesChangeMarks().files[path]; cf != nil && (cf.Untracked || cf.Status == gitworkbench.StatusAdded) {
		isNew = true
	}
	return rel, isNew, true
}

// canAnnotate reports whether the open file is a tracked text file in the code
// view: the only kind Plain | Blame and History speak about.
func (s *Shell) canAnnotate(selected string) bool {
	st := &s.rightPanel
	if st.previewPlanned != selected || st.nativeCode == nil || s.git == nil {
		return false
	}
	if _, ok := s.repoRel(selected); !ok {
		return false
	}
	cf := s.filesChangeMarks().files[selected]
	return cf == nil || !(cf.Untracked || cf.Status == gitworkbench.StatusAdded)
}

// filesGitControls is the preview header's right end: Plain | Blame and
// History.
func (s *Shell) filesGitControls(c *ui.Context, selected string) {
	if !s.canAnnotate(selected) {
		return
	}
	g := &s.filesGit
	choice := 0
	if g.blame {
		choice = 1
	}
	if segmentedControl(c, &choice, "Plain", "Blame").Label("Annotate").Changed() {
		g.blame = choice == 1
		if g.blame {
			s.beginBlame(selected)
		}
	}
	rel, _ := s.repoRel(selected)
	history := regularButton(c, "History")
	if tipped(c, history, "The commits that touched this file").Clicked() {
		s.openFileSheet("history", rel, "")
	}
}

// beginBlame reads the file's blame in the background; the lines land on the
// UI lane unless the user moved to another file meanwhile.
func (s *Shell) beginBlame(path string) {
	rel, ok := s.repoRel(path)
	if !ok || s.git == nil {
		return
	}
	g := &s.filesGit
	g.blameGen++
	gen := g.blameGen
	g.blameFor, g.blameLines, g.blameErr, g.blameLoading = path, nil, "", true
	g.blameList = ui.ListState{}
	root := s.git.root
	runner := s.git.runner
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), gitworkbench.ReadTimeout*2)
		defer cancel()
		lines, err := runner.Blame(ctx, root, "", rel)
		s.applyOnUI(func() {
			if gen != g.blameGen {
				return
			}
			g.blameLoading = false
			if err != nil {
				g.blameErr = gitFailureText(err)
				return
			}
			g.blameLines = lines
		})
	}()
}

// blameOn reports whether the preview shows the Blame view for selected.
func (s *Shell) blameOn(selected string) bool {
	return s.filesGit.blame && s.canAnnotate(selected)
}

// uncommittedHash is what git blame calls a line the working tree wrote.
const uncommittedHash = "0000000000000000000000000000000000000000"

// filesBlameBody is the Blame view: per line the commit that wrote it (its
// author, hash and age on the first line of a block), the line number and
// the text.
func (s *Shell) filesBlameBody(c *ui.Context, selected string) {
	g := &s.filesGit
	if g.blameFor != selected {
		s.beginBlame(selected)
	}
	t := c.Theme()
	switch {
	case g.blameLoading:
		ui.Column(c).Grow(1).Center().Children(func() { ui.Spinner(c).Size(16, 16) })
		return
	case g.blameErr != "":
		emptyState(c, "Can't annotate this file", g.blameErr)
		return
	case len(g.blameLines) == 0:
		emptyState(c, "Nothing to annotate", "The file has no lines yet.")
		return
	}
	tokens := designTokens(t.Dark)
	ui.List(c, &g.blameList, len(g.blameLines), func(i int) {
		if i < 0 || i >= len(g.blameLines) {
			return
		}
		bl := g.blameLines[i]
		first := i == 0 || g.blameLines[i-1].Hash != bl.Hash
		row := ui.Row(c).FillWidth().Height(blameRowH).Padding(0, 8, 0, 4).Gap(8).AlignItems(ui.Center).MinWidth(0)
		if row.Hovered() {
			row.Background(tokens.SidebarHover)
		}
		fresh := bl.Hash == uncommittedHash
		if !fresh && row.Clicked() {
			s.revealCommit(bl.Hash)
		}
		if first {
			summary := "Not committed yet"
			if !fresh {
				summary = bl.Summary + "\n" + bl.Author + " · " + gdCommitTime(bl.Time)
			}
			tipped(c, row, summary)
		}
		row.Children(func() {
			ui.Row(c).Width(blameColumnW).Shrink(0).Gap(6).AlignItems(ui.Center).Children(func() {
				if !first {
					return
				}
				if fresh {
					ui.Box(c).Size(14, 14).Shrink(0).Radius(7).Background(t.TextMuted.Alpha(0.35)).Center().Children(func() {
						ui.Icon(c, iconEdit).Size(9, 9).TextColor(t.Text).PassThrough()
					})
					ui.Text(c, "Not committed yet").FontSize(Typography().BodySmall).TextColor(t.TextMuted).SingleLine().
						Ellipsis("…").MinWidth(0)
					return
				}
				authorAvatar(c, bl.Author, 14)
				ui.Text(c, bl.Short).Font(gdCodeFont()).FontSize(Typography().BodySmall).TextColor(t.Accent).Shrink(0).SingleLine()
				ui.Text(c, gdRelativeTime(bl.Time)).FontSize(Typography().BodySmall).TextColor(t.TextMuted).SingleLine().
					Ellipsis("…").MinWidth(0)
			})
			ui.Text(c, itoa(bl.Line)).Font(gdCodeFont()).FontSize(Typography().BodySmall).TextColor(t.TextMuted).
				Width(40).Shrink(0).SingleLine()
			ui.Text(c, bl.Text).Font(gdCodeFont()).FontSize(Typography().BodySmall).Grow(1).MinWidth(0).SingleLine()
		})
	}).Grow(1).MinHeight(0)
}

// The Blame view's annotation column and the height every row shares, with or
// without an avatar, so the lines keep one rhythm.
const (
	blameColumnW = 196
	blameRowH    = 18
)

// filesPreviewFoot is the Files preview's foot.
func (s *Shell) filesPreviewFoot(c *ui.Context) { s.codeFoot(c, s.rightPanel.nativeCode) }

// codeFoot is a code view's foot: the gutter's legend while the file is drawn
// against HEAD, and the language, encoding and line count.
func (s *Shell) codeFoot(c *ui.Context, code *editor.State) {
	if code == nil || code.Buffer == nil {
		return
	}
	t := c.Theme()
	th := codeEditorTheme(t)
	muted := t.TextMuted
	typ := Typography()
	ui.Row(c).FillWidth().Padding(Spacing().S, Spacing().XL).Gap(Spacing().L).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(designTokens(t.Dark).BorderSubtle).Children(func() {
		if code.DiffBase != nil {
			for _, key := range []struct {
				label string
				color ui.Color
			}{{"Added", th.Added}, {"Modified", th.Modified}, {"Removed", th.Deleted}} {
				ui.Row(c).Gap(5).AlignItems(ui.Center).Shrink(0).Children(func() {
					ui.Box(c).Size(3, 10).Radius(1.5).Background(key.color).Shrink(0)
					ui.Text(c, key.label).FontSize(typ.Caption).TextColor(muted)
				})
			}
			ui.Text(c, "since HEAD").FontSize(typ.Caption).TextColor(muted)
		}
		ui.Spacer(c)
		language := "Plain text"
		if lang := code.Buffer.Language(); lang != nil && lang.Name != "" {
			language = lang.Name
		}
		lines := textLineCount(code.Buffer)
		noun := " lines"
		if lines == 1 {
			noun = " line"
		}
		ui.Text(c, language+" · UTF-8 · "+itoa(lines)+noun).FontSize(typ.Caption).TextColor(muted).SingleLine()
	})
}

// textLineCount is the number of lines a file reads as: the buffer's own count
// minus the empty one after a final newline, and 0 for an empty file.
func textLineCount(b *editor.Buffer) int {
	n := b.LineCount()
	if b.Len() == 0 {
		return 0
	}
	if n > 1 && b.Line(n-1) == "" {
		n--
	}
	return n
}
