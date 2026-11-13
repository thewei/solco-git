package nativeui

/**
 * [INPUT]: 提交审阅（gd_*.go 的文件卡与行号列）、gitworkbench 的 PatchSource / PatchLinesFor / RewriteWithPatch / PublishedCommits
 * [OUTPUT]: Custom Patch 的选择（整文件：文件菜单；按行：点行号，⇧ 连选、⌘ 追加，拖过去也行）、审阅顶部的 Custom Patch 条（Remove / Split / Move / Undo / Clear）、Move 的目标提交选择、确认面板
 * [POS]: 2026-11-12 决定第 (12) 条：以文件或「行」为粒度改写历史。选择记在界面坐标（hunk, 行）里，执行时才用 PatchSource 换成 LazyGit 的行号（对不上就拒绝，改选整个文件）；失败（含冲突）由内核中止并还原
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

// patchFileSel is what is chosen of one file of the commit under review:
// all of it, or the changed lines picked on the diff as {hunk, line}.
type patchFileSel struct {
	whole bool
	lines map[[2]int]bool
}

// patchSelection is the Custom Patch choice. It belongs to one commit.
type patchSelection struct {
	commit string
	files  map[string]*patchFileSel
	// anchor is {hunk, line} where a ⇧ range starts, in anchorPath.
	anchor     [2]int
	anchorPath string
	anchored   bool
}

// patchDestState is the "move to which commit" picker.
type patchDestState struct {
	open    bool
	commits []gitworkbench.CommitInfo
	list    ui.ListState
}

func (s *Shell) clearPatchSelection() { s.patch = patchSelection{} }

// patchEligible says whether the commit under review can be rewritten with a
// Custom Patch, and when not, why: it must be an ordinary commit of the
// checked-out branch, not a merge, a stash or a comparison.
func (s *Shell) patchEligible() (bool, string) {
	if s.git == nil || !s.gdReviewingCommit() || s.git.compare {
		return false, ""
	}
	hash := s.git.commitHash
	if slices.ContainsFunc(s.git.stashes, func(st gitworkbench.Stash) bool { return st.Hash == hash }) {
		return false, "A stash is not part of the branch"
	}
	if s.git.commitMeta != nil && len(s.git.commitMeta.Parents) > 1 {
		return false, "A merge commit cannot be edited line by line"
	}
	if merged := s.commitView().merged; merged != nil && !merged[hash] {
		return false, "This commit is not on the checked-out branch"
	}
	return true, ""
}

// gdPatchPickable reports whether the file's changed lines can be picked for
// a Custom Patch: a text file of an eligible commit with hunks to pick from.
func (s *Shell) gdPatchPickable(f *gdFile) bool {
	if ok, _ := s.patchEligible(); !ok {
		return false
	}
	cf := f.cf
	return !cf.Binary && !cf.TooLarge && !cf.Deferred && len(f.hunks()) > 0
}

// patchSel returns the commit's selection of a file, making one when asked.
func (s *Shell) patchSel(path string, create bool) *patchFileSel {
	if s.patch.commit != s.git.commitHash {
		if !create {
			return nil
		}
		s.patch = patchSelection{commit: s.git.commitHash}
	}
	sel := s.patch.files[path]
	if sel == nil && create {
		if s.patch.files == nil {
			s.patch.files = map[string]*patchFileSel{}
		}
		sel = &patchFileSel{}
		s.patch.files[path] = sel
	}
	return sel
}

func (s *Shell) patchDrop(path string) {
	delete(s.patch.files, path)
	if len(s.patch.files) == 0 {
		s.clearPatchSelection()
	}
}

// patchPicked reports whether a line of the file is chosen (a whole-file
// choice picks every changed line).
func (s *Shell) patchPicked(f *gdFile, hunk, index int) bool {
	sel := s.patchSel(f.cf.Path, false)
	return sel != nil && (sel.whole || sel.lines[[2]int{hunk, index}])
}

// patchPickedIn returns the picked lines of one hunk, nil for none.
func (s *Shell) patchPickedIn(f *gdFile, hunk int) map[int]bool {
	sel := s.patchSel(f.cf.Path, false)
	if sel == nil {
		return nil
	}
	if sel.whole {
		return gdChangedLines(f.hunks()[hunk])
	}
	var out map[int]bool
	for k := range sel.lines {
		if k[0] == hunk {
			if out == nil {
				out = map[int]bool{}
			}
			out[k[1]] = true
		}
	}
	return out
}

// patchPickLine answers a click on a changed line's number, like the staging
// pick: plain picks just that line (again, to clear), ⌘ toggles, ⇧ ranges.
func (s *Shell) patchPickLine(f *gdFile, hunk, index int, mods ui.Modifiers) {
	path := f.cf.Path
	sel := s.patchSel(path, true)
	if sel.whole {
		// A line picked out of a whole-file choice narrows it to that line.
		sel.whole, sel.lines = false, nil
	}
	if sel.lines == nil {
		sel.lines = map[[2]int]bool{}
	}
	key := [2]int{hunk, index}
	p := &s.patch
	switch {
	case mods&ui.Shift != 0 && p.anchored && p.anchorPath == path && p.anchor[0] == hunk:
		s.patchPickRange(f, hunk, p.anchor[1], index)
		return
	case mods&ui.Cmd != 0:
		if sel.lines[key] {
			delete(sel.lines, key)
		} else {
			sel.lines[key] = true
		}
	case len(sel.lines) == 1 && sel.lines[key]:
		clear(sel.lines)
	default:
		clear(sel.lines)
		sel.lines[key] = true
	}
	p.anchor, p.anchorPath, p.anchored = [2]int{hunk, index}, path, true
	if len(sel.lines) == 0 {
		s.patchDrop(path)
	}
}

// patchPickRange picks exactly the changed lines of one hunk between two line
// indices.
func (s *Shell) patchPickRange(f *gdFile, hunk, from, to int) {
	sel := s.patchSel(f.cf.Path, true)
	sel.whole = false
	sel.lines = map[[2]int]bool{}
	lo, hi := min(from, to), max(from, to)
	for i, l := range f.hunks()[hunk].Lines {
		if i >= lo && i <= hi && l.Kind != gitworkbench.KindContext {
			sel.lines[[2]int{hunk, i}] = true
		}
	}
}

// patchSetHunk picks, or clears, every changed line of one hunk.
func (s *Shell) patchSetHunk(f *gdFile, hunk int, on bool) {
	sel := s.patchSel(f.cf.Path, true)
	if sel.whole {
		sel.whole, sel.lines = false, nil
		for hi, h := range f.hunks() {
			for li := range gdChangedLines(h) {
				if sel.lines == nil {
					sel.lines = map[[2]int]bool{}
				}
				sel.lines[[2]int{hi, li}] = true
			}
		}
	}
	if sel.lines == nil {
		sel.lines = map[[2]int]bool{}
	}
	for li := range gdChangedLines(f.hunks()[hunk]) {
		if on {
			sel.lines[[2]int{hunk, li}] = true
		} else {
			delete(sel.lines, [2]int{hunk, li})
		}
	}
	if len(sel.lines) == 0 {
		s.patchDrop(f.cf.Path)
	}
}

// togglePatchFile chooses a whole file, or takes it out again.
func (s *Shell) togglePatchFile(cf *gitworkbench.ChangeFile) {
	if ok, _ := s.patchEligible(); !ok {
		return
	}
	if sel := s.patchSel(cf.Path, false); sel != nil && sel.whole {
		s.patchDrop(cf.Path)
		return
	}
	sel := s.patchSel(cf.Path, true)
	sel.whole, sel.lines = true, nil
}

// patchItems adds the file menu's Custom Patch entry.
func (s *Shell) patchItems(m *ui.Menu, cf *gitworkbench.ChangeFile) {
	ok, why := s.patchEligible()
	if why == "" && !ok {
		return
	}
	whole := false
	if sel := s.patchSel(cf.Path, false); sel != nil {
		whole = sel.whole
	}
	label := "Add File to Custom Patch"
	if whole {
		label = "Remove File from Custom Patch"
	}
	if m.Item(label).Disabled(!ok || s.git.opBusy).Chosen() {
		s.togglePatchFile(cf)
	}
}

// patchCounts returns how many files and lines (a whole file counts as one
// file and no lines) are chosen.
func (s *Shell) patchCounts() (files, lines int) {
	if s.patch.commit != s.git.commitHash {
		return 0, 0
	}
	for _, sel := range s.patch.files {
		files++
		lines += len(sel.lines)
	}
	return files, lines
}

// patchBadge is the short note on a file card: what of it is in the patch.
func (s *Shell) patchBadge(path string) string {
	sel := s.patchSel(path, false)
	switch {
	case sel == nil:
		return ""
	case sel.whole:
		return "In patch"
	}
	return gdCountOf(len(sel.lines), "line") + " in patch"
}

// patchBar is the strip above a commit's diff while something is chosen.
func (s *Shell) patchBar(c *ui.Context) {
	if ok, _ := s.patchEligible(); !ok {
		return
	}
	files, lines := s.patchCounts()
	if files == 0 {
		return
	}
	t := c.Theme()
	busy := s.git.opBusy
	text := pluralFiles(files)
	if lines > 0 {
		text += ", " + gdCountOf(lines, "line")
	}
	ui.Row(c).FillWidth().Padding(6, 12).Gap(8).AlignItems(ui.Center).Background(t.Accent.Alpha(0.10)).Children(func() {
		ui.Text(c, "Custom Patch").FontSize(Typography().BodySmall).FontWeight(WeightSemibold).SingleLine().Shrink(0)
		ui.Text(c, text).FontSize(Typography().BodySmall).TextColor(t.TextMuted).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
		ui.Spacer(c)
		tip := func(b ui.Element, text string) ui.Element { return tipped(c, b, text) }
		if tip(compactButton(c, "Remove…").Disabled(busy), "Remove from Commit: delete these changes from the commit").Clicked() {
			s.openPatchSheet(gitworkbench.PatchRemove, "")
		}
		if tip(compactButton(c, "Split…").Disabled(busy), "Split into New Commit: make them a commit of their own, right after this one").Clicked() {
			s.openPatchSheet(gitworkbench.PatchSplit, "")
		}
		if tip(compactButton(c, "Move…").Disabled(busy), "Move to Commit: move them into another commit of the branch").Clicked() {
			s.openPatchDest()
		}
		if tip(compactButton(c, "Undo…").Disabled(busy), "Undo to Working Tree: take them out of the commit and put them back, staged").Clicked() {
			s.openPatchSheet(gitworkbench.PatchUndo, "")
		}
		if compactButton(c, "Clear").Clicked() {
			s.clearPatchSelection()
		}
	})
}

// patchHunkRow is a commit-review hunk's slim header with its pick actions.
func (s *Shell) patchHunkRow(c *ui.Context, pal *gdPalette, f *gdFile, r *gdRow) {
	hunks := f.hunks()
	if int(r.hunk) >= len(hunks) {
		ui.Box(c).Height(0)
		return
	}
	h := hunks[r.hunk]
	picked := s.patchPickedIn(f, int(r.hunk))
	row := s.gdCard(c, pal).Height(gdHunkRowH).Background(pal.gapBg).AlignItems(ui.Center)
	row.Children(func() {
		ui.Text(c, gdHunkHeaderText(h)).Font(gdCodeFont()).FontSize(Typography().BodySmall).TextColor(pal.gapText).
			SingleLine().Grow(1).MinWidth(0).Margin(0, 12).Unselectable()
		if !row.Hovered() && len(picked) == 0 {
			return
		}
		ui.Row(c).Shrink(0).Gap(6).AlignItems(ui.Center).Padding(0, 8, 0, 0).Children(func() {
			if len(picked) > 0 {
				if compactButton(c, "Clear").Clicked() {
					s.patchSetHunk(f, int(r.hunk), false)
				}
				return
			}
			tip := "Add every change in this hunk to the Custom Patch. Click line numbers to pick lines (⇧ for a range, ⌘ to add)"
			if tipped(c, compactButton(c, "Add Hunk to Patch").Disabled(s.git.opBusy), tip).Clicked() {
				s.patchSetHunk(f, int(r.hunk), true)
			}
		})
	})
}

// patchSummary is the sheet's file list.
func (s *Shell) patchSummary() []historyLine {
	var paths []string
	for p := range s.patch.files {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	out := make([]historyLine, 0, len(paths))
	for _, p := range paths {
		sel := s.patch.files[p]
		tag := "whole file"
		if !sel.whole {
			tag = gdCountOf(len(sel.lines), "line")
		}
		out = append(out, historyLine{tag: tag, text: p})
	}
	return out
}

var patchVerbs = map[gitworkbench.PatchAction][3]string{
	gitworkbench.PatchRemove: {"Remove from Commit?", "Remove", "These changes are deleted from the commit, and the commits above it are replayed on top (new hashes). A later commit that touched the same lines conflicts; then nothing changes."},
	gitworkbench.PatchSplit:  {"Split into a New Commit?", "Create Commit", "These changes become a commit of their own right after the original, and the commits above are replayed on top (new hashes). If anything conflicts, nothing changes."},
	gitworkbench.PatchMove:   {"Move to Another Commit?", "Move", "These changes leave this commit and join the one below, and the commits in between are replayed (new hashes). If anything conflicts, nothing changes."},
	gitworkbench.PatchUndo:   {"Undo to the Working Tree?", "Undo", "These changes leave the commit and come back in the working tree, staged, and the commits above are replayed on top (new hashes). If anything conflicts, nothing changes."},
}

// openPatchSheet opens the confirm sheet of a Custom Patch operation; dest is
// the commit a move goes into.
func (s *Shell) openPatchSheet(action gitworkbench.PatchAction, dest string) {
	if s.git == nil || s.git.opBusy {
		return
	}
	if files, _ := s.patchCounts(); files == 0 {
		return
	}
	commit := s.git.commitHash
	short := commit
	if s.git.commitMeta != nil {
		short = s.git.commitMeta.Short
	}
	verb := patchVerbs[action]
	// The hunks the selection was made on: the mapping to Git's line numbers
	// runs in the background against exactly these.
	shown := map[string][]gitworkbench.Hunk{}
	prev := map[string]string{}
	for _, f := range s.git.gdFiles {
		shown[f.cf.Path] = f.hunks()
		if f.cf.OldPath != "" && f.cf.OldPath != f.cf.Path {
			prev[f.cf.Path] = f.cf.OldPath
		}
	}
	sel := map[string]patchFileSel{}
	for p, v := range s.patch.files {
		sel[p] = patchFileSel{whole: v.whole, lines: maps.Clone(v.lines)}
	}
	lines := append([]historyLine{{tag: "from", text: short}}, s.patchSummary()...)
	needsMsg := action == gitworkbench.PatchSplit
	s.readHistory("Custom Patch", func(ctx context.Context, root string) (historySheetState, error) {
		hashes := []string{commit}
		if dest != "" {
			hashes = append(hashes, dest)
		}
		published, _ := s.git.runner.PublishedCommits(ctx, root, hashes)
		return historySheetState{
			title:    verb[0],
			intro:    verb[2] + historyUndoHint,
			warn:     publishedWarning(published),
			confirm:  verb[1],
			lines:    lines,
			field:    "Message of the new commit",
			needsMsg: needsMsg,
			name:     "Custom Patch",
			run: func(ctx context.Context, root, msg string) (string, error) {
				op, err := buildPatchOp(ctx, s.git.runner, root, action, commit, dest, msg, sel, shown, prev)
				if err != nil {
					return "", err
				}
				if err := s.git.runner.RewriteWithPatch(ctx, root, op); err != nil {
					return "", err
				}
				s.afterHistoryRewrite()
				return "Custom Patch applied", nil
			},
		}, nil
	})
}

// buildPatchOp turns the choice, made on the shown diff, into the operation:
// whole files as they are, picked lines through Git's own reading of the
// file's diff (a mismatch refuses the line choice).
func buildPatchOp(ctx context.Context, r *gitworkbench.Runner, root string, action gitworkbench.PatchAction, commit, dest, msg string,
	sel map[string]patchFileSel, shown map[string][]gitworkbench.Hunk, prev map[string]string) (gitworkbench.PatchOp, error) {
	paths := make([]string, 0, len(sel))
	for p := range sel {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	op := gitworkbench.PatchOp{Action: action, Commit: commit, Dest: dest, Message: strings.TrimSpace(msg)}
	for _, p := range paths {
		f := sel[p]
		file := gitworkbench.PatchFile{Path: p, PreviousPath: prev[p]}
		if f.whole {
			file.Whole = true
		} else {
			src, err := r.PatchSource(ctx, root, commit, p, prev[p])
			if err != nil {
				return op, err
			}
			if file.Lines, err = gitworkbench.PatchLinesFor(src, shown[p], f.lines); err != nil {
				return op, err
			}
		}
		op.Files = append(op.Files, file)
	}
	return op, nil
}

// openPatchDest lists the commits a move can go into: the branch's other
// commits, newest first.
func (s *Shell) openPatchDest() {
	if s.git == nil || s.git.opBusy {
		return
	}
	if files, _ := s.patchCounts(); files == 0 {
		return
	}
	merged := s.commitView().merged
	var cands []gitworkbench.CommitInfo
	for _, c := range s.git.commits {
		if c.Hash != s.git.commitHash && (merged == nil || merged[c.Hash]) && len(c.Parents) <= 1 {
			cands = append(cands, c)
		}
	}
	if len(cands) == 0 {
		s.pendingToast = "Move to Commit: there is no other commit on this branch"
		return
	}
	s.gitMore.patchDest = patchDestState{open: true, commits: cands}
}

func (s *Shell) patchDestSheet(c *ui.Context) {
	d := &s.gitMore.patchDest
	if !d.open {
		return
	}
	t := c.Theme()
	ui.Modal(c, &d.open, func() {
		ui.Column(c).Width(620).MaxWidth(780).Height(460).MinHeight(240).MinWidth(0).Gap(Spacing().M).Children(func() {
			modalTitle(c, "Move to Which Commit?")
			ui.Text(c, "The chosen changes join the commit you click. A commit older than this one takes them as an addition, a newer one only if they still apply there.").
				FontSize(Typography().BodySmall).TextColor(t.TextMuted).MaxWidth(580)
			ui.List(c, &d.list, len(d.commits), func(i int) {
				if i < 0 || i >= len(d.commits) {
					return
				}
				cm := d.commits[i]
				row := ui.Row(c).Gap(8).Padding(4, 8).Radius(treeRowRadius).AlignItems(ui.Center).MinWidth(0)
				if row.Hovered() {
					row.Background(designTokens(t.Dark).SidebarHover)
				}
				if row.Clicked() {
					d.open = false
					s.openPatchSheet(gitworkbench.PatchMove, cm.Hash)
				}
				row.Children(func() {
					ui.Text(c, cm.Short).Font(gdCodeFont()).FontSize(Typography().BodySmall).TextColor(t.TextMuted).Width(62).SingleLine()
					ui.Text(c, cm.Subject).FontSize(Typography().Body).Grow(1).MinWidth(0).SingleLine()
				})
			}).Grow(1).MinHeight(0)
			ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
				if regularButton(c, "Cancel").Clicked() {
					d.open = false
				}
			})
		})
	})
}
