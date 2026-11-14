package nativeui

/**
 * [INPUT]: gitworkbench 的 CourierPatch / PreviewCourier / DeliverCourier / DiscardCourierLines、侧栏已读好的 worktree 列表、Changes 的文件菜单与行选择、提交审阅的 Custom Patch 选择
 * [OUTPUT]: Change Courier：把一个文件、选中的行或 Custom Patch 选中的内容送到同一仓库的另一个 worktree——选目标、先预检（干净才可送）、送达只进目标工作区且不暂存；可选 Move（成功后从源丢弃：工作区整文件、工作区行、或从提交里 Remove）
 * [POS]: 2026-11-12 决定第 (13) 条的界面一侧：预检与送达绑同一份冻结的补丁；换目标即作废旧预检；送达走目标仓库的 Safety Journal（那边 ⌥⌘Z 可退回）
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

// courierState is the "Send to Worktree" sheet. The patch is made once, when
// the sheet opens, and every preview and the delivery use that text.
type courierState struct {
	open     bool
	title    string // what is being sent, in words
	srcs     []gitworkbench.CourierSource
	patch    string
	building bool
	problem  string // the patch could not be made
	dests    []gitworkbench.Worktree
	pick     int
	preview  *gitworkbench.CourierPreview
	checking bool
	err      string // the preview could not be read
	token    int    // bumped by every new patch or destination: older answers are void
	move     bool
	moveOK   bool
	moveWhy  string
	remove   func(ctx context.Context, root string) error
	list     ui.ListState
}

// courierDestinations lists the repository's other worktrees a patch can go to.
func (s *Shell) courierDestinations() []gitworkbench.Worktree {
	cur, _ := os.Stat(s.git.root)
	var out []gitworkbench.Worktree
	for _, w := range s.git.worktrees {
		if w.Bare {
			continue
		}
		info, err := os.Stat(w.Path)
		if err != nil || !info.IsDir() || (cur != nil && os.SameFile(cur, info)) {
			continue
		}
		out = append(out, w)
	}
	return out
}

func courierLabel(w gitworkbench.Worktree) string {
	switch {
	case w.Branch != "":
		return w.Branch
	case w.Detached && len(w.Head) >= 7:
		return "detached @ " + w.Head[:7]
	}
	return filepath.Base(w.Path)
}

// canCourier says whether sending is possible at all, and why not.
func (s *Shell) canCourier() (bool, string) {
	if s.git == nil || s.git.opBusy {
		return false, "Another Git operation is running"
	}
	if len(s.courierDestinations()) == 0 {
		return false, "There is no other worktree to send to; create one first (Worktrees in the sidebar)"
	}
	return true, ""
}

// openCourier opens the sheet for the sources and starts making their patch.
func (s *Shell) openCourier(title string, srcs []gitworkbench.CourierSource, moveOK bool, moveWhy string, remove func(ctx context.Context, root string) error) {
	if ok, why := s.canCourier(); !ok {
		s.pendingToast = "Send to Worktree: " + why
		return
	}
	c := &s.gitMore.courier
	token := c.token + 1
	*c = courierState{open: true, title: title, srcs: srcs, building: true, dests: s.courierDestinations(), pick: -1,
		token: token, moveOK: moveOK, moveWhy: moveWhy, remove: remove}
	root, runner := s.git.root, s.git.runner
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), gitworkbench.ReadTimeout*3)
		defer cancel()
		patch, err := runner.CourierPatch(ctx, root, srcs...)
		s.applyOnUI(func() {
			if s.gitMore.courier.token != token || !s.gitMore.courier.open {
				return
			}
			c := &s.gitMore.courier
			c.building = false
			if err != nil {
				c.problem = gitFailureText(err)
				return
			}
			c.patch = patch
			s.courierPickDest(0)
		})
	}()
}

// courierPickDest chooses a destination and checks the patch against it.
func (s *Shell) courierPickDest(i int) {
	c := &s.gitMore.courier
	if i < 0 || i >= len(c.dests) || c.patch == "" {
		return
	}
	c.token++
	token, dest, patch, runner := c.token, c.dests[i].Path, c.patch, s.git.runner
	c.pick, c.preview, c.checking, c.err = i, nil, true, ""
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), gitworkbench.ReadTimeout*3)
		defer cancel()
		pre, err := runner.PreviewCourier(ctx, dest, patch)
		s.applyOnUI(func() {
			c := &s.gitMore.courier
			if c.token != token || !c.open {
				return
			}
			c.checking = false
			if err != nil {
				c.err = gitFailureText(err)
				return
			}
			c.preview = &pre
		})
	}()
}

// courierRemoveWhole is Move's removal of a whole working-tree file from the
// source: an untracked file is deleted, a tracked one restored from the index.
func (s *Shell) courierRemoveWhole(cf *gitworkbench.ChangeFile) func(ctx context.Context, root string) error {
	path, untracked, runner := cf.Path, cf.Untracked, s.git.runner
	return func(ctx context.Context, root string) error {
		if untracked {
			return runner.DeleteUntracked(root, []string{path})
		}
		return runner.Discard(ctx, root, []string{path})
	}
}

// openCourierFile sends one file of the working tree (the file menu).
func (s *Shell) openCourierFile(cf *gitworkbench.ChangeFile) {
	if cf.Status == gitworkbench.StatusConflicted {
		s.pendingToast = "Send to Worktree: resolve the conflicts in " + cf.Path + " first"
		return
	}
	renamed := cf.OldPath != "" && cf.OldPath != cf.Path
	moveOK, why := true, ""
	switch {
	case renamed:
		moveOK, why = false, "A renamed file is sent as a copy; move it by hand"
	case cf.Staged:
		moveOK, why = false, "Part of it is staged; unstage it first to move it"
	}
	s.openCourier(cf.Path, []gitworkbench.CourierSource{{Path: cf.Path, OldPath: cf.OldPath}}, moveOK, why, s.courierRemoveWhole(cf))
}

// openCourierLines sends the lines picked in one hunk of the shown side.
func (s *Shell) openCourierLines(f *gdFile, hunk int, picked map[int]bool) {
	picks := map[[2]int]bool{}
	for i := range picked {
		picks[[2]int{hunk, i}] = true
	}
	hunks := slices.Clone(f.hunks())
	src := gitworkbench.CourierSource{Path: f.cf.Path, Hunks: hunks, Picks: picks}
	unstage, _ := s.gdStageSide(f)
	moveOK, why := !unstage, ""
	if unstage {
		why = "These lines are staged; unstage them first to move them"
	}
	runner := s.git.runner
	s.openCourier(gdCountOf(len(picked), "line")+" of "+f.cf.Path, []gitworkbench.CourierSource{src}, moveOK, why,
		func(ctx context.Context, root string) error { return runner.DiscardCourierLines(ctx, root, src) })
}

// openCourierPatch sends the Custom Patch choice of the commit under review.
func (s *Shell) openCourierPatch() {
	if ok, _ := s.patchEligible(); !ok {
		return
	}
	files, _ := s.patchCounts()
	if files == 0 {
		return
	}
	commit := s.git.commitHash
	shown := map[string][]gitworkbench.Hunk{}
	prev := map[string]string{}
	for _, f := range s.git.gdFiles {
		shown[f.cf.Path] = slices.Clone(f.hunks())
		if f.cf.OldPath != "" && f.cf.OldPath != f.cf.Path {
			prev[f.cf.Path] = f.cf.OldPath
		}
	}
	sel := map[string]patchFileSel{}
	var srcs []gitworkbench.CourierSource
	paths := make([]string, 0, len(s.patch.files))
	for p := range s.patch.files {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	for _, p := range paths {
		v := s.patch.files[p]
		sel[p] = patchFileSel{whole: v.whole, lines: maps.Clone(v.lines)}
		src := gitworkbench.CourierSource{Path: p, OldPath: prev[p], Commit: commit}
		if !v.whole {
			src.Hunks, src.Picks = shown[p], maps.Clone(v.lines)
		}
		srcs = append(srcs, src)
	}
	runner := s.git.runner
	title := pluralFiles(files) + " of " + shortHash(commit)
	if s.git.commitMeta != nil {
		title = pluralFiles(files) + " of " + s.git.commitMeta.Short
	}
	s.openCourier(title, srcs, true, "", func(ctx context.Context, root string) error {
		op, err := buildPatchOp(ctx, runner, root, gitworkbench.PatchRemove, commit, "", "", sel, shown, prev)
		if err != nil {
			return err
		}
		if err := runner.RewriteWithPatch(ctx, root, op); err != nil {
			return err
		}
		s.afterHistoryRewrite()
		return nil
	})
}

// courierDeliver sends the frozen patch to the chosen worktree (and, for a
// Move, takes the changes out of the source after they have arrived).
func (s *Shell) courierDeliver() {
	c := &s.gitMore.courier
	if !c.open || c.pick < 0 || c.preview == nil || !c.preview.Clean {
		return
	}
	dest, patch, move, remove, runner := c.dests[c.pick], c.patch, c.move && c.moveOK, c.remove, s.git.runner
	c.open = false
	name := "Send to Worktree"
	if move {
		name = "Move to Worktree"
	}
	s.runGitOp(name, func(ctx context.Context, root string) (string, error) {
		if err := runner.DeliverCourier(ctx, dest.Path, patch); err != nil {
			return "", err
		}
		if move {
			if err := remove(ctx, root); err != nil {
				return "", fmt.Errorf("the changes arrived in %s, but they could not be taken out here: %s", courierLabel(dest), gitFailureText(err))
			}
			return "Moved to " + courierLabel(dest), nil
		}
		return "Sent to " + courierLabel(dest) + " — unstaged there, and unchanged here", nil
	})
}

func (s *Shell) courierSheet(c *ui.Context) {
	st := &s.gitMore.courier
	if !st.open {
		return
	}
	t := c.Theme()
	ui.Modal(c, &st.open, func() {
		ui.Column(c).Width(620).MaxWidth(780).Gap(Spacing().M).Children(func() {
			modalTitle(c, "Send to Another Worktree")
			ui.Text(c, st.title).Font(gdCodeFont()).FontSize(Typography().Body).SingleLine().Ellipsis("…").MaxWidth(580)
			ui.Text(c, "The changes arrive in that worktree's files, unstaged. Nothing is committed or staged there, and Undo Git Action (⌥⌘Z) in it takes them back.").
				FontSize(Typography().BodySmall).TextColor(t.TextMuted).MaxWidth(580)
			switch {
			case st.building:
				busyNote(c, "Reading the changes…")
			case st.problem != "":
				ui.Text(c, st.problem).FontSize(Typography().BodySmall).TextColor(t.Danger).MaxWidth(580)
			default:
				s.courierDestinationList(c, st)
				s.courierPreviewPane(c, st)
				if st.moveOK {
					ui.Checkbox(c, &st.move, "Move: take these changes out of here once they have arrived")
				} else if st.moveWhy != "" {
					quietNote(c, "Move is off: "+st.moveWhy)
				}
			}
			ui.Row(c).FillWidth().Gap(Spacing().M).Justify(ui.End).Children(func() {
				if regularButton(c, "Cancel").Clicked() {
					st.open = false
				}
				ready := st.preview != nil && st.preview.Clean && !s.git.opBusy
				label := "Send"
				if st.move && st.moveOK {
					label = "Move"
				}
				if regularPrimaryButton(c, label).Disabled(!ready).Clicked() {
					s.courierDeliver()
				}
			})
		})
	})
}

func (s *Shell) courierDestinationList(c *ui.Context, st *courierState) {
	t := c.Theme()
	ui.Text(c, "Send to").FontSize(Typography().BodySmall).FontWeight(WeightSemibold).TextColor(t.TextMuted)
	ui.List(c, &st.list, len(st.dests), func(i int) {
		if i < 0 || i >= len(st.dests) {
			return
		}
		w := st.dests[i]
		row := ui.Row(c).FillWidth().Gap(10).Padding(5, 8).Radius(treeRowRadius).AlignItems(ui.Center).MinWidth(0)
		switch {
		case i == st.pick:
			row.Background(designTokens(t.Dark).SidebarSelect)
		case row.Hovered():
			row.Background(designTokens(t.Dark).SidebarHover)
		}
		if row.Clicked() && i != st.pick {
			s.courierPickDest(i)
		}
		row.Children(func() {
			ui.Icon(c, iconWorktree).Size(Controls().IconRegular, Controls().IconRegular).Shrink(0).TextColor(t.TextMuted)
			ui.Text(c, courierLabel(w)).FontSize(Typography().Body).FontWeight(WeightMedium).SingleLine().Shrink(0)
			ui.Text(c, tildePath(w.Path)).Font(gdCodeFont()).FontSize(Typography().BodySmall).TextColor(t.TextMuted).
				SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
		})
	}).MaxHeight(150).MinHeight(0)
}

func (s *Shell) courierPreviewPane(c *ui.Context, st *courierState) {
	t := c.Theme()
	switch {
	case st.checking:
		busyNote(c, "Checking that it fits…")
	case st.err != "":
		ui.Text(c, st.err).FontSize(Typography().BodySmall).TextColor(t.Danger).MaxWidth(580)
	case st.preview == nil:
	case !st.preview.Clean:
		ui.Text(c, "It does not fit there, so nothing can be sent:").FontSize(Typography().BodySmall).FontWeight(WeightSemibold).TextColor(t.Warning)
		ui.Text(c, st.preview.Problem).Font(gdCodeFont()).FontSize(Typography().BodySmall).TextColor(t.TextMuted).MaxWidth(580)
	default:
		ui.Text(c, "Fits cleanly — "+pluralFiles(len(st.preview.Files))).FontSize(Typography().BodySmall).FontWeight(WeightSemibold).TextColor(t.Success)
		for _, f := range st.preview.Files {
			ui.Row(c).Gap(10).Children(func() {
				ui.Text(c, "+"+itoa(f.Added)+" −"+itoa(f.Removed)).Font(gdCodeFont()).FontSize(Typography().BodySmall).TextColor(t.TextMuted).Width(80).Shrink(0)
				ui.Text(c, f.Path).Font(gdCodeFont()).FontSize(Typography().BodySmall).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
			})
		}
	}
}

// courierItem is the file menu's entry for the working tree.
func (s *Shell) courierItem(m *ui.Menu, cf *gitworkbench.ChangeFile) {
	ok, _ := s.canCourier()
	if m.Item("Send to Worktree…").Disabled(!ok || cf.Status == gitworkbench.StatusConflicted).Chosen() {
		s.openCourierFile(cf)
	}
}
