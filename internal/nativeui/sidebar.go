package nativeui

import (
	"cmp"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

// sidebarFilesView renders staged/unstaged file lists in the UPPER CENTER
// Git pane. The left sidebar manages repository references independently.
func (s *Shell) sidebarFilesView(c *ui.Context) {
	t := c.Theme()
	pal := gdPaletteFor(t)

	snap := s.gdActiveSnapshot()
	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		s.gitListHead(c, &headerSearch{
			Query: &s.changes.filter, Open: &s.changesSearchOpen, Label: "Filter files", Button: "Search files",
			Changed: func() {
				s.changesTree = nil
				s.git.gdRowsDirty = true
			},
		}, 10, func() {
			s.changesSummary(c, snap)
		})

		if s.git.root == "" {
			ui.Column(c).Grow(1).Center().Padding(16).Children(func() {
				ui.Text(c, "No Git repository").FontSize(Typography().Body).TextColor(t.TextMuted)
			})
			return
		}
		if snap == nil {
			ui.Column(c).Grow(1).Center().Padding(16).Gap(6).Children(func() {
				if s.git.refreshing {
					ui.Spinner(c).Size(16, 16)
				}
				ui.Text(c, "Reading changes…").FontSize(Typography().Body).TextColor(t.TextMuted)
			})
			return
		}

		// A clean tree: the center's "Working tree clean" says it once; two
		// empty Unstaged/Staged headers would say it again.
		if len(snap.Files) == 0 {
			return
		}
		unstagedFiles, stagedFiles := s.splitSections(snap)
		unstagedRows := s.sectionRows(unstagedFiles)
		stagedRows := s.sectionRows(stagedFiles)
		// Files leave a pane as they are staged, unstaged or filtered away;
		// a choice keyed by a path the pane no longer shows must not quietly
		// feed the pane's action.
		pruneChoice(&s.unstagedChoice, unstagedFiles)
		pruneChoice(&s.stagedChoice, stagedFiles)

		lists := ui.Column(c).Grow(1).MinHeight(0)
		lists.Children(func() {
			// The Unstaged pane (untracked files included) takes the space
			// above the divider. The pane action targets the Cmd/Shift
			// selection when there is one, else the whole pane.
			s.gdSectionHeader(c, "Unstaged Files", len(unstagedFiles), &s.unstagedOpen,
				"Stage", gdActionPaths(&s.unstagedChoice, unstagedFiles), s.stageFiles)
			if s.unstagedOpen && len(unstagedRows) > 0 {
				s.gdChangedTreeList(c, pal, unstagedRows, &s.unstagedList, &s.unstagedSelected,
					&s.unstagedChoice, gdViewUnstaged, func(row gitworkbench.FlatRow) { s.changesClickSide(row, gdViewUnstaged) })
			}

			if s.unstagedOpen && s.stagedOpen && len(stagedFiles) > 0 && len(unstagedFiles) > 0 {
				s.gdPaneDivider(c)
			}

			// The Staged pane sits below the divider at the dragged height.
			s.gdSectionHeader(c, "Staged Files", len(stagedFiles), &s.stagedOpen,
				"Unstage", gdActionPaths(&s.stagedChoice, stagedFiles), s.unstageFiles)
			if s.stagedOpen && len(stagedRows) > 0 {
				list := s.gdChangedTreeList(c, pal, stagedRows, &s.stagedList, &s.stagedSelected,
					&s.stagedChoice, gdViewStaged, func(row gitworkbench.FlatRow) { s.changesClickSide(row, gdViewStaged) })
				if s.unstagedOpen && len(unstagedFiles) > 0 {
					// Basis, not Height: the list's Grow(1) gave it a 0% basis that
					// Height does not override, so both lists shared the room equally
					// and the divider moved nothing (2026-11-08 run).
					list.Basis(s.stagedListHeight(lists.Bounds().H, len(stagedRows))).Grow(0)
				} else {
					list.Grow(1).MinHeight(0)
				}
			}
		})
	})
}

// The widths of the center under which the Commits list drops its author and
// then its age column.
const (
	gitCommitsShowAuthor float32 = 760
	gitCommitsShowAge    float32 = 600
)

// commitsPane is the Commits pane: the history of HEAD, filtered by the
// search field, each row opening the commit's diff in the review.
func (s *Shell) commitsPane(c *ui.Context, pal *gdPalette) {
	t := c.Theme()

	// A search reads the whole window; otherwise folded merges leave their
	// merged-in branch out of both the list and the graph.
	view := s.commitView()
	fold, foldedCounts := view.fold, view.counts
	entries := view.shown
	filtering := strings.TrimSpace(s.gitCommitFilter) != "" || s.commitFilterOn()
	if filtering {
		entries = s.filteredCommits()
	}
	if s.gdReviewingCommit() {
		s.commitsSelected = -1
		for i, cm := range entries {
			if cm.Hash == s.git.commitHash {
				s.commitsSelected = i
			}
		}
	}

	// The graph column hugs the lanes the history uses; a filtered list shows
	// dots only, since its rows are no longer neighbours.
	windowW, _ := c.Size()
	centerW := s.gitCenterWidth(windowW)
	graphByHash := view.graphs
	focused := s.commitsList.FocusWithin(c)
	hiddenRefs := s.graphHiddenSet()
	chipIndex := s.newGitChipIndex()
	s.consumeLocate(entries)
	if s.git.commitsLoading && len(entries) == 0 {
		ui.Column(c).Grow(1).Center().Padding(16).Children(func() {
			ui.Spinner(c).Size(16, 16)
		})
		return
	}
	// Keyed before the list is built: the entries may have grown or shrunk
	// since the last frame (fold, unfold, search), and a stale key function
	// would give the new rows no key.
	s.commitsList.Key = func(i int) any {
		if i >= 0 && i < len(entries) {
			return entries[i].Hash
		}
		return nil
	}
	list := ui.List(c, &s.commitsList, len(entries), func(i int) {
		if i < 0 || i >= len(entries) {
			return
		}
		cm := entries[i]
		current := s.gdReviewingCommit() && s.git.commitHash == cm.Hash
		// Padding carries no vertical inset on purpose: the graph glyph is
		// the row's pitch, and rows must tile edge to edge for the lane
		// wires to run unbroken (the compact All Commits list, 2026-10-14).
		row := ui.Row(c).Gap(commitRowGap).Padding(0, 8).Radius(treeRowRadius).AlignItems(ui.Center).MinWidth(0)
		muted, ref := t.TextMuted, pal.ref
		// halo is the row's own paper, wash included: the graph rings its
		// dot and its lines with it.
		halo := t.Background
		// A commit the checked-out branch does not contain (another branch's
		// work, not merged in) rests in the muted colour, Fork-style.
		unmerged := view.merged != nil && !view.merged[cm.Hash]
		subjectColor := t.Text
		if unmerged {
			subjectColor, ref = t.TextMuted, t.TextMuted
		}
		switch {
		case s.git.flashHash == cm.Hash:
			// The commit a sidebar click just located.
			row.Background(pal.match).TextColor(t.Text)
			halo = pal.match.Over(t.Background)
			muted, ref = t.TextMuted, t.Text
		case s.commitChosen(cm.Hash):
			// the ⌘ / ⇧ choice a history rewrite acts on
			row.Background(designTokens(t.Dark).SidebarSelectStrong).TextColor(t.Text)
			halo = designTokens(t.Dark).SidebarSelectStrong.Over(t.Background)
			muted, ref = t.TextMuted, t.Text
		case current && focused:
			row.Background(designTokens(t.Dark).SidebarSelectStrong).TextColor(t.Text)
			halo = designTokens(t.Dark).SidebarSelectStrong.Over(t.Background)
			muted, ref = t.TextMuted, t.Text
		case current:
			// Unfocused selection shares the sidebar's token so the same
			// "this row is selected" tint reads identically across the
			// tree, the commits list and the changes tree (F155).
			row.Background(designTokens(t.Dark).SidebarSelect)
			halo = designTokens(t.Dark).SidebarSelect.Over(t.Background)
		case row.Hovered():
			row.Background(designTokens(t.Dark).SidebarHover)
			halo = designTokens(t.Dark).SidebarHover.Over(t.Background)
		}
		if row.Clicked() {
			if s.pickCommit(entries, cm.Hash, row.ClickModifiers()) {
				s.reviewCommit(cm.Hash)
			}
		}
		if row.DoubleClicked() {
			s.checkoutCommitRow(cm)
		}
		// Long commit subjects preserve the full metadata in the tooltip.
		tip := cm.Subject + "\n" + cm.Author + " · " + gdCommitTime(cm.Time) + "\n" + cm.Hash
		if unmerged {
			tip += "\nNot merged into the current branch"
		}
		tipped(c, row, tip)
		row.ContextMenu(func(m *ui.Menu) {
			if chosen := s.commitChoiceHashes(s.git.commits); len(chosen) > 1 && s.commitChosen(cm.Hash) {
				s.commitBatchMenu(m, chosen)
				return
			}
			if m.Item("Cherry-pick Commit…").Chosen() {
				s.confirmCherryPickCommit(cm.Hash, cm.Subject)
			}
			if m.Item("Revert Commit…").Chosen() {
				s.confirmRevertCommit(cm.Hash, cm.Subject)
			}
			m.Separator()
			if m.Item("Interactive Rebase Onto Here…").Disabled(s.git.opBusy).Chosen() {
				s.openRebaseSheet(cm.Hash, cm.Short)
			}
			s.rebaseFromMenu(m, cm)
			s.commitRewriteMenu(m, cm)
			m.Submenu("Reset Current Branch to Here", func(sub *ui.Menu) {
				for _, mode := range []gitworkbench.ResetMode{gitworkbench.ResetSoft, gitworkbench.ResetMixed, gitworkbench.ResetHard} {
					if sub.Item(resetModeText[mode][0] + "…").Disabled(s.git.opBusy).Chosen() {
						s.confirmResetTo(cm.Hash, cm.Subject, mode)
					}
				}
			})
			m.Separator()
			if m.Item("Compare to Local Changes").Chosen() {
				s.compareToWorktree(cm.Hash)
			}
			if m.Item("Save as Patch…").Chosen() {
				s.savePatch(cm.Hash, cm.Subject)
			}
			m.Separator()
			if m.Item("New Branch Here…").Disabled(s.git.opBusy).Chosen() {
				s.openCreateAt("git-create-branch-at", cm.Hash)
			}
			if m.Item("New Tag Here…").Disabled(s.git.opBusy).Chosen() {
				s.openTagDialog(cm.Hash)
			}
			if m.Item("Check Out This Commit…").Disabled(s.git.opBusy).Chosen() {
				s.confirmCheckoutCommit(cm.Hash, cm.Subject)
			}
			m.Separator()
			s.commitLinkItems(m, cm.Hash)
			if m.Item("Copy Commit Hash").Chosen() {
				s.copyToClipboard(cm.Hash)
			}
			if m.Item("Copy Subject").Chosen() {
				s.copyToClipboard(cm.Subject)
			}
		})
		row.Children(func() {
			graph := graphByHash[cm.Hash]
			graphW := gitGraphRowWidth(graph, view.lanes)
			if filtering {
				graphW = gitGraphDotsWidth(view.lanes, 1)
			}
			gitGraphGlyph(c, graph, view.lanes, graphW, halo, filtering)
			gitCommitChips(c, chipIndex.chips(cm, hiddenRefs), ref, func(chip gitChip) {
				// A chip opens its commit and shows where its ref lives in
				// the sidebar.
				s.reviewCommit(cmp.Or(chip.review, cm.Hash))
				s.revealInSidebar(chip.target)
			})
			gitCommitSubject(c, cm.Subject, subjectColor)
			if !filtering && fold.foldable(cm.Hash) {
				s.foldToggle(c, cm.Hash, foldedCounts[cm.Hash])
			}
			// Author and age give way first when the center is narrow.
			if centerW >= gitCommitsShowAuthor {
				ui.Row(c).Width(commitAuthorColW).Shrink(0).Gap(5).AlignItems(ui.Center).Children(func() {
					authorAvatar(c, cm.Author, commitAvatarSize)
					ui.Text(c, cm.Author).FontSize(Typography().BodySmall).TextColor(muted).Grow(1).MinWidth(0).SingleLine()
				})
			}
			ui.Text(c, cm.Short).Font(gdCodeFont()).FontSize(Typography().BodySmall).TextColor(ref).Width(commitHashColW).SingleLine()
			if centerW >= gitCommitsShowAge {
				ui.Text(c, gdRelativeTime(cm.Time)).FontSize(Typography().BodySmall).TextColor(muted).Width(commitDateColW).SingleLine()
			}
		})
	}).Grow(1).MinHeight(0).Padding(4, 8)
	list.Children(func() {
		if len(entries) == 0 {
			quietNote(c, "No matching commits")
		}
	})
}

// gdRelativeTime writes how long ago t was: just now, 5m ago, 3d ago.
func gdRelativeTime(t time.Time) string {
	d := timeNow().Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d/time.Hour))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d/(24*time.Hour)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(d/(30*24*time.Hour)))
	}
	return fmt.Sprintf("%dy ago", int(d/(365*24*time.Hour)))
}

// splitSections partitions the snapshot's files into the unstaged pane
// (worktree changes and untracked files) and the staged pane.
func (s *Shell) splitSections(snap *gitworkbench.ChangesSnapshot) (unstaged, staged []gitworkbench.ChangeFile) {
	for i := range snap.Files {
		cf := snap.Files[i]
		switch {
		case cf.Untracked:
			if !s.changes.hideUntracked {
				unstaged = append(unstaged, cf)
			}
		default:
			if cf.Unstaged {
				unstaged = append(unstaged, cf)
			}
			if cf.Staged {
				staged = append(staged, cf)
			}
		}
	}
	return unstaged, staged
}

// sectionRows builds the collapsible tree rows of one pane, honoring the
// shared filter and directory-collapse state.
func (s *Shell) sectionRows(files []gitworkbench.ChangeFile) []gitworkbench.FlatRow {
	tree := gitworkbench.FilterTree(gitworkbench.BuildTree(files, true), s.changes.filter)
	return gitworkbench.Flatten(tree, s.git.collapsedDirs)
}

// panePaths lists a pane's file paths in snapshot order.
func panePaths(files []gitworkbench.ChangeFile) []string {
	paths := make([]string, 0, len(files))
	for _, cf := range files {
		paths = append(paths, cf.Path)
	}
	return paths
}

// pruneChoice drops chosen paths the pane no longer shows, so a staged,
// unstaged or filtered-away file cannot linger in its old pane's choice.
func pruneChoice(choice *ui.Selection[string], files []gitworkbench.ChangeFile) {
	if choice.Len() == 0 {
		return
	}
	shown := make(map[string]bool, len(files))
	for _, cf := range files {
		shown[cf.Path] = true
	}
	for path := range choice.All() {
		if !shown[path] {
			choice.Remove(path)
		}
	}
}

// gdActionPaths returns the paths a pane's Stage/Unstage action targets:
// the Cmd/Shift selection when there is one, else every file of the pane.
func gdActionPaths(choice *ui.Selection[string], files []gitworkbench.ChangeFile) []string {
	if choice.Len() == 0 {
		return panePaths(files)
	}
	paths := make([]string, 0, choice.Len())
	for _, cf := range files {
		if choice.Has(cf.Path) {
			paths = append(paths, cf.Path)
		}
	}
	return paths
}

// gdSectionHeader is one pane's title row: the shared tree header (chevron,
// semibold title, count) with the pane's Stage/Unstage action kept visible at
// its right edge.
func (s *Shell) gdSectionHeader(c *ui.Context, title string, count int, open *bool, actionLabel string, paths []string, action func([]string)) {
	spec := sidebarSectionSpec{Key: "changes:" + title, Title: title, Count: count, Closed: !*open}
	if count > 0 && !s.git.opBusy {
		spec.Trailing = func() {
			if sectionTextAction(c, actionLabel, actionLabel+" ("+pluralFiles(len(paths))+")").Clicked() {
				action(paths)
			}
		}
	}
	if _, clicked := s.sidebarSectionHeader(c, spec); clicked {
		*open = !*open
	}
}

// gdPaneDivider is the draggable line between the two panes: dragging it
// hands the space to the pane above or below. Dragging down grows the
// unstaged pane above, so the staged pane below shrinks by the same delta.
// The drag starts from the height the Staged list actually has, so the line
// follows the pointer from the first pixel (2026-11-14: it used to start from
// a stored height the list never reached, and the first stretch of a drag
// moved nothing).
func (s *Shell) gdPaneDivider(c *ui.Context) {
	drag := splitHandle(c, s.tokens(c.Theme().Dark), splitHorizontal, "Resize panes")
	switch {
	case drag.Reset:
		s.stagedPaneHeight, s.stagedPaneSized = stagedPaneDefault, false
	case drag.Held && drag.DY != 0: // a plain click on the line is not a size
		s.stagedPaneHeight = max(s.stagedPaneShown-drag.DY, 60)
		s.stagedPaneSized = true
	}
	if s.splitFinished("staged", drag) {
		s.savePaneSizes()
	}
}

// stagedListHeight is the Staged list's height. By default it is the stored
// one, no taller than its own rows plus the list's 3-point padding top and
// bottom, so a short list leaves the room to the Unstaged rows above. Once the
// user has dragged the divider the dragged height stands (blank space and all:
// they chose it). Either way it is held between 60 points and what leaves the
// Unstaged list 60 points and both headers, as the previous frame laid the
// column out (written back, so a drag past it leaves no dead stretch to drag
// back through). Before a first layout there is no column height and the
// stored one stands.
func (s *Shell) stagedListHeight(listsH float32, rows int) float32 {
	if listsH > 0 {
		s.stagedPaneHeight = clampF(s.stagedPaneHeight, 60, max(listsH-stagedPaneReserve, 60))
	}
	h := s.stagedPaneHeight
	if !s.stagedPaneSized {
		h = min(h, float32(rows)*s.treeRowHeight()+6)
	}
	s.stagedPaneShown = h
	return h
}

// stagedPaneDefault is the Staged list's stored height before any drag.
const stagedPaneDefault float32 = 220

// stagedPaneReserve is what the Staged list leaves the rest of the column: the
// Unstaged list's 60-point floor, the two section headers and the divider.
const stagedPaneReserve float32 = 60 + 2*30 + splitHitWidth

// clampF bounds a pane dimension.
func clampF(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// changesSummary is the list head's one summary of a changed-file list — the
// file count, the added / removed totals, "partial" when the list was cut —
// shared by the working tree's and a commit's file column so the two heads read
// the same (2026-11-14 UX pass: a commit's head said "Changed Files 6" in a
// heavier weight beside the working tree's muted "16 files +16 -0").
func (s *Shell) changesSummary(c *ui.Context, snap *gitworkbench.ChangesSnapshot) {
	if snap == nil {
		return
	}
	t := c.Theme()
	pal := gdPaletteFor(t)
	ui.Text(c, pluralFiles(len(snap.Files))).FontSize(Typography().BodySmall).TextColor(t.TextMuted).SingleLine()
	tipped(c, ui.Row(c).Gap(6).Shrink(0), "Total change: "+gdLines(snap.TotalAdditions, "added")+", "+gdLines(snap.TotalDeletions, "removed")).Children(func() {
		ui.Text(c, "+"+gdThousands(snap.TotalAdditions)).Font(gdCodeFont()).FontSize(Typography().BodySmall).FontWeight(WeightSemibold).TextColor(pal.addText)
		ui.Text(c, "-"+gdThousands(snap.TotalDeletions)).Font(gdCodeFont()).FontSize(Typography().BodySmall).FontWeight(WeightSemibold).TextColor(pal.delText)
	})
	if snap.Truncated {
		ui.Text(c, "partial").FontSize(Typography().Micro).TextColor(t.TextMuted)
	}
}
