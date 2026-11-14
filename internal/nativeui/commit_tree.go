package nativeui

/**
 * [INPUT]: 依赖 Shell 的当前审阅提交（git.commitMeta）与 Runner（ListTree/ReadBlob）、sidebar_tree.go 的树行、files_git.go 的代码页脚、MyGo editor
 * [OUTPUT]: 对外提供 commitTreeState、commitScopeTabs（Changes | Tree）、commitTreeList（该提交下整棵树，目录按需列）、commitTreeFileView（选中文件在那一刻的只读代码页）
 * [POS]: Fork 的 File Tree：Commits 视图里，左栏可以从"这个提交改了什么"切到"这个提交那一刻的整棵树"，右栏相应从 Diff 换成只读的文件；全部读自 git ls-tree / cat-file，不写任何东西，不碰工作区
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"bytes"
	"context"
	"path"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/thewei/solco-gitkit/filesview"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

// commitTreeState is the Tree mode of the commit review. The listing is the
// tree of one commit: it is dropped whenever another commit is reviewed.
type commitTreeState struct {
	on    bool
	hash  string                              // the commit the listing belongs to
	dirs  map[string][]gitworkbench.TreeEntry // listed folders, by repository path ("" is the root)
	asked map[string]bool                     // folders listed or being listed
	open  map[string]bool
	err   string

	selected string // repository path of the open file
	file     commitTreeFile
	gen      int // bumped when the commit changes or another file is chosen: older reads are dropped
}

// commitTreeFile is the file open at that commit.
type commitTreeFile struct {
	size    int64
	loading bool
	note    string // why there is no code to show ("Binary file", …)
	detail  string
	code    *editor.State
}

// commitTreeSync points the state at the commit under review; another commit
// drops the listing and the open file, the chosen tab stays.
func (s *Shell) commitTreeSync(hash string) {
	ts := &s.commitTree
	if ts.hash == hash && ts.dirs != nil {
		return
	}
	*ts = commitTreeState{
		on: ts.on, hash: hash, gen: ts.gen + 1,
		dirs: map[string][]gitworkbench.TreeEntry{}, asked: map[string]bool{}, open: map[string]bool{},
	}
	s.loadTreeDir("")
}

// loadTreeDir lists one folder of the commit's tree in the background.
func (s *Shell) loadTreeDir(dir string) {
	ts := &s.commitTree
	if ts.asked[dir] || s.git == nil || ts.hash == "" {
		return
	}
	ts.asked[dir] = true
	gen, hash := ts.gen, ts.hash
	root, runner := s.git.root, s.git.runner
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), gitworkbench.ReadTimeout)
		defer cancel()
		entries, err := runner.ListTree(ctx, root, hash, dir)
		s.applyOnUI(func() {
			if gen != ts.gen {
				return
			}
			if err != nil {
				ts.err = gitFailureText(err)
				return
			}
			if entries == nil {
				entries = []gitworkbench.TreeEntry{}
			}
			ts.dirs[dir] = entries
		})
	}()
}

// commitScopeTabs is the review's left column head: what the commit changed,
// or the whole tree as it stood.
func (s *Shell) commitScopeTabs(c *ui.Context) {
	ts := &s.commitTree
	chosen := 0
	if ts.on {
		chosen = 1
	}
	ui.Row(c).FillWidth().Padding(8, 10, 2).Children(func() {
		if segmentedControl(c, &chosen, "Changes", "Tree").Label("Commit files").Changed() {
			ts.on = chosen == 1
		}
	})
}

// commitTreeList draws the commit's tree: folders open on demand, a file
// opens read-only in the right pane.
func (s *Shell) commitTreeList(c *ui.Context) {
	ts := &s.commitTree
	ui.Column(c).Grow(1).MinHeight(0).MinWidth(0).Children(func() {
		switch {
		case ts.err != "":
			emptyState(c, "Can't read this commit's tree", ts.err)
		case ts.dirs[""] == nil:
			ui.Column(c).Grow(1).Center().Children(func() { ui.Spinner(c).Size(15, 15) })
		default:
			ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
				s.navList(c, &s.nav.commitTree, false, ui.Column(c).FillWidth().Padding(4, sidebarGutter, 12), func() {
					s.renderCommitTree(c, "", 0)
				})
			})
		}
	})
}

func (s *Shell) renderCommitTree(c *ui.Context, dir string, level int) {
	ts := &s.commitTree
	entries := ts.dirs[dir]
	if dir != "" && entries == nil {
		ui.Row(c).FillWidth().Height(s.treeRowHeight()).Padding(0, 0, 0, treeGlyphX(level)).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c).Size(12, 12)
		})
		return
	}
	for _, entry := range entries {
		e := entry
		switch {
		case e.Dir:
			open := ts.open[e.Path]
			_, clicked := s.sidebarTreeRow(c, sidebarTreeSpec{
				Key: "tree:" + e.Path, Title: e.Name, Label: "Folder " + e.Name, Tooltip: e.Path, Level: level,
				Folder: true, Expanded: open, Glyph: iconFolder, Nav: &s.nav.commitTree, Motion: true,
			})
			if clicked {
				ts.open[e.Path] = !open
				if !open {
					s.loadTreeDir(e.Path)
				}
			}
			if open {
				s.renderCommitTree(c, e.Path, level+1)
			}
		case e.Submodule:
			s.sidebarTreeRow(c, sidebarTreeSpec{
				Key: "tree:" + e.Path, Title: e.Name, Label: "Submodule " + e.Name, Tooltip: "Submodule — another repository", Level: level,
				Glyph: iconFolder, Dim: true, Nav: &s.nav.commitTree,
			})
		default:
			spec := sidebarTreeSpec{
				Key: "tree:" + e.Path, Title: e.Name, Label: "File " + e.Name, Tooltip: e.Path, Level: level,
				Glyph: iconFile, Nav: &s.nav.commitTree, Motion: true,
				Current: ts.selected == e.Path, Selected: ts.selected == e.Path,
				Menu: func(m *ui.Menu) {
					if m.Item("Copy Path").Chosen() {
						s.copyToClipboard(e.Path)
					}
					if m.Item("File History").Chosen() {
						s.openFileSheet("history", e.Path, "")
					}
				},
			}
			if _, clicked := s.sidebarTreeRow(c, spec); clicked {
				s.openTreeFile(e)
			}
		}
	}
}

// openTreeFile reads the file as it stood at the commit, in the background.
func (s *Shell) openTreeFile(e gitworkbench.TreeEntry) {
	ts := &s.commitTree
	if s.git == nil || ts.hash == "" {
		return
	}
	ts.gen++
	gen, hash := ts.gen, ts.hash
	ts.selected = e.Path
	ts.file = commitTreeFile{size: e.Size, loading: true}
	root, runner := s.git.root, s.git.runner
	name := path.Base(e.Path)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), gitworkbench.ReadTimeout*2)
		defer cancel()
		data, err := runner.ReadBlob(ctx, root, hash+":"+e.Path, filesview.TextPreviewCap)
		var code *editor.State
		note, detail := "", ""
		switch {
		case err != nil:
			note, detail = "Can't read this file", gitFailureText(err)
			if ge, ok := gitworkbench.GitError(err); ok && ge.Class == gitworkbench.ErrTooLarge {
				note, detail = "Too large to show", filesview.FormatFileSize(e.Size)+" — open it from the working tree instead."
			}
		case bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0:
			note, detail = "Binary file", filesview.FormatFileSize(e.Size)
		default:
			buf := editor.NewBuffer(string(data))
			buf.SetLanguage(editor.LanguageFor(name))
			code = &editor.State{
				Buffer: buf, ReadOnly: true, NoActiveLine: true, Label: name, Placeholder: "Empty file",
				SoftWrap: buf.Language() == nil,
			}
		}
		s.applyOnUI(func() {
			if gen != ts.gen {
				return
			}
			ts.file = commitTreeFile{size: e.Size, note: note, detail: detail, code: code}
		})
	}()
}

// commitTreeFileView is the right pane of Tree mode: the chosen file, read
// only, headed by the commit it is read from.
func (s *Shell) commitTreeFileView(c *ui.Context) {
	ts := &s.commitTree
	t := c.Theme()
	typ := Typography()
	sp := Spacing()
	short := ts.hash
	if cm := s.git.commitMeta; cm != nil {
		short = cm.Short
	}
	ui.Column(c).Grow(1).MinHeight(0).MinWidth(0).Children(func() {
		if ts.selected == "" {
			emptyState(c, "Choose a file", "It opens here as it stood in "+short+", read-only.")
			return
		}
		ui.Row(c).FillWidth().MinHeight(Controls().RegularHeight+2*sp.S).Padding(sp.S, sp.XL).Gap(sp.M).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(designTokens(t.Dark).BorderSubtle).Children(func() {
			ui.Icon(c, iconFile).Size(Controls().IconRegular, Controls().IconRegular).TextColor(t.TextMuted).Shrink(0)
			tipped(c, ui.Text(c, path.Base(ts.selected)).FontSize(typ.Body).FontWeight(WeightSemibold).
				Shrink(1).MinWidth(0).SingleLine(), ts.selected)
			folder := ""
			if dir := path.Dir(ts.selected); dir != "." {
				folder = dir
			}
			ui.Text(c, folder).FontSize(typ.Caption).TextColor(t.TextMuted).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
			if ts.file.size > 0 {
				ui.Text(c, filesview.FormatFileSize(ts.file.size)).FontSize(typ.Caption).TextColor(t.TextMuted)
			}
			statusPill(c, "At "+short, ToneNeutral)
		})
		switch {
		case ts.file.loading:
			ui.Column(c).Grow(1).Center().Children(func() { ui.Spinner(c).Size(16, 16) })
		case ts.file.note != "":
			emptyState(c, ts.file.note, ts.file.detail)
		case ts.file.code != nil:
			ts.file.code.Theme = codeEditorTheme(t)
			editor.View(c, ts.file.code).FillWidth().Grow(1).MinHeight(0)
			s.codeFoot(c, ts.file.code)
		}
	})
}

// treeAt reports whether the review is in Tree mode: the tab is chosen and a
// commit (not a comparison with the working tree) is under review.
func (s *Shell) treeAt() bool {
	return s.commitTree.on && s.gdReviewingCommit() && !s.git.compare
}
