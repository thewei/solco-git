package nativeui

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

// gitSectionAction is a section's one creation action: the "+" that shows
// under the pointer and the same entry in the section's context menu.
type gitSectionAction struct {
	label    string // accessibility label of the "+" button
	menuItem string // context-menu title
	disabled bool
	run      func()
	// more adds items to the section's context menu below the creation item.
	more func(m *ui.Menu)
}

// segmentTab is one segment of segmentTabs: a glyph and a label.
type segmentTab struct {
	label string
	svg   *ui.SVG
}

// segmentTabs is the two-way switch under the Files search (Files | Content):
// the sidebar's segmented face (materialFace — it sits on the vibrancy
// material) with an icon on each segment. badge, when given, returns a count
// shown after a segment's label (0 shows none). It reports whether the choice
// changed.
func segmentTabs(c *ui.Context, name string, chosen *int, items []segmentTab, badge func(i int) int) bool {
	t := c.Theme()
	tabs := ui.SegmentedBase(c, chosen, len(items))
	materialSegmentedTrack(c, tabs, name)
	tabs.Track.FillWidth().Children(func() {
		for i, tab := range items {
			seg, ink := materialSegmentFacePadded(c, tabs, i, i == *chosen, tab.label, t.Space(3))
			seg.Grow(1).Justify(ui.Center)
			seg.Children(func() {
				ui.Icon(c, tab.svg).Size(gorexPaneGlyph, gorexPaneGlyph).TextColor(ink).Shrink(0)
				ui.Text(c, tab.label).SingleLine().TextColor(ink)
				if badge != nil {
					if n := badge(i); n > 0 {
						ui.Text(c, itoa(n)).FontSize(Typography().BodySmall).TextColor(sidebarMuted(t)).SingleLine()
					}
				}
			})
		}
	})
	return tabs.Track.Changed()
}

// gitRefsFilterChanged starts a new search expanded, without carrying stale
// search-scoped folds into a different result set.
func (s *Shell) gitRefsFilterChanged() {
	for key := range s.gitTreeClosed {
		if strings.HasPrefix(key, "search/") {
			delete(s.gitTreeClosed, key)
		}
	}
}

func (s *Shell) gitRepositorySidebar(c *ui.Context) {
	ui.Column(c).Grow(1).MinHeight(0).Padding(4, sidebarGutter, 0).Gap(6).Children(func() {
		if s.git.root == "" {
			quietNote(c, "No Git repository")
			return
		}
		if !s.git.refsRead {
			// Every section would read "0" until the first read lands.
			busyNote(c, "Reading references…")
			return
		}
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			s.navList(c, &s.nav.gitRefs, false, ui.Column(c).FillWidth().Padding(0, 0, 12), func() {
				s.gitSidebarSection(c, "Worktrees", len(s.git.worktrees), &gitSectionAction{
					label: "Add worktree", menuItem: "Add Worktree…", disabled: s.git.opBusy,
					run: func() {
						s.openTextDialog("git-add-worktree", "", "Add Worktree",
							"Absolute path (a branch named after the folder is created from HEAD)", "")
					},
				}, func() {
					shown := 0
					for _, wt := range s.git.worktrees {
						label := gitWorktreeLabel(wt)
						if !gitRefMatches(s.gitRefsFilter, label, wt.Path) {
							continue
						}
						shown++
						worktree := wt
						item := gitTreeItem{title: label, tooltip: wt.Path, kind: "Worktree", glyph: iconWorktree}
						switch {
						case wt.Bare:
						case wt.Branch != "":
							item.ref = gitworkbench.RefHeadsPrefix + wt.Branch
						case wt.Head != "":
							item.commit = wt.Head
						}
						s.gitTreeRow(c, item, func() {
							if !worktree.Bare {
								s.openExternalPath(worktree.Path)
							}
						}, func(m *ui.Menu) {
							s.worktreeMenu(m, worktree, worktree.Path == s.git.worktrees[0].Path)
						})
					}
					s.gitEmptySection(c, shown)
				})
				s.gitSidebarSection(c, "Branches", len(s.git.branches), &gitSectionAction{
					label: "New branch", menuItem: "New Branch…", disabled: s.git.branchBusy,
					run: s.openNewBranchDialog,
				}, func() {
					refs := make([]string, 0, len(s.git.branches))
					for _, b := range s.git.branches {
						refs = append(refs, b.Name)
					}
					s.gitRenderRefTree(c, "local", gitRefTree(refs, s.gitRefsFilter), 0, func(full string) {
						for _, branch := range s.git.branches {
							if branch.Name == full && !branch.Current {
								s.confirmSwitchBranch(full)
								return
							}
						}
					}, func(full string, m *ui.Menu) {
						var current bool
						for _, branch := range s.git.branches {
							if branch.Name == full {
								current = branch.Current
								break
							}
						}
						if !current {
							if m.Item("Switch to Branch").Chosen() {
								s.confirmSwitchBranch(full)
							}
							if m.Item("Merge into Current…").Chosen() {
								s.openMergeDialog(full)
							}
							if m.Item("Rebase Current Onto This…").Disabled(s.git.opBusy).Chosen() {
								s.confirmRebaseOnto(full)
							}
							m.Separator()
						} else if s.git.upstream.OK {
							// What Fork's current-branch menu offers beside the toolbar.
							if m.Item("Pull (Fast-Forward Only)").Disabled(s.git.opBusy).Chosen() {
								s.pullRepo()
							}
							if m.Item("Push").Disabled(s.git.opBusy || s.git.upstream.Diverged()).Chosen() {
								s.pushRepo()
							}
							m.Separator()
						}
						if m.Item("Rename Branch…").Disabled(s.git.opBusy).Chosen() {
							s.openRenameBranch(full)
						}
						s.trackingMenu(m, full)
						if !current && m.Item("Delete Branch…").Chosen() {
							s.deleteBranch(full)
						}
						m.Separator()
						if m.Item("Copy Branch Name").Chosen() {
							s.copyToClipboard(full)
						}
					})
				})
				s.gitSidebarSection(c, "Remotes", len(s.git.remoteBranches)+len(s.remotesWithoutBranches()), &gitSectionAction{
					label: "Add remote", menuItem: "Add Remote…", disabled: s.git.opBusy,
					run: func() { s.openRemoteDialog("add", "") },
				}, func() {
					refs := make([]string, 0, len(s.git.remoteBranches))
					for _, b := range s.git.remoteBranches {
						refs = append(refs, b.Remote+"/"+b.Name)
					}
					empties := s.remotesWithoutBranches()
					for _, rm := range empties {
						if !gitRefMatches(s.gitRefsFilter, rm.Name, rm.FetchURL) {
							continue
						}
						remote := rm
						s.gitTreeRow(c, gitTreeItem{title: remote.Name, tooltip: remote.FetchURL, kind: "Remote", glyph: iconCloud, badge: "not fetched"},
							func() {}, func(m *ui.Menu) { s.remoteMenu(m, remote.Name) })
					}
					if len(refs) == 0 {
						if len(empties) == 0 {
							s.gitEmptySection(c, 0)
						}
						return
					}
					s.gitRenderRefTree(c, "remote", gitRefTree(refs, s.gitRefsFilter), 0, func(full string) {
						s.copyToClipboard(full)
						s.pendingToast = "Copied remote-tracking ref"
					}, func(full string, m *ui.Menu) {
						if m.Item("Check Out…").Disabled(s.git.opBusy).Chosen() {
							s.checkoutRemoteBranch(full)
						}
						if s.currentBranchName() != "" {
							if m.Item("Merge into Current…").Disabled(s.git.opBusy).Chosen() {
								s.openMergeDialog(full)
							}
							if m.Item("Rebase Current Onto This…").Disabled(s.git.opBusy).Chosen() {
								s.confirmRebaseOnto(full)
							}
						}
						m.Separator()
						if m.Item("Delete Remote Branch…").Disabled(s.git.opBusy).Chosen() {
							s.confirmDeleteRemoteBranch(full)
						}
						if m.Item("Copy Remote-Tracking Ref").Chosen() {
							s.copyToClipboard(full)
						}
					})
				})
				s.gitSidebarSection(c, "Tags", len(s.git.tags), &gitSectionAction{
					label: "New tag", menuItem: "New Tag…", disabled: s.git.opBusy,
					run: func() { s.openTagDialog("") },
				}, func() {
					refs := make([]string, 0, len(s.git.tags))
					for _, tag := range s.git.tags {
						refs = append(refs, tag.Name)
					}
					s.gitRenderRefTree(c, "tag", gitRefTree(refs, s.gitRefsFilter), 0, func(full string) {
						s.copyToClipboard(full)
					}, func(full string, m *ui.Menu) {
						if m.Item("Push Tag").Disabled(s.git.opBusy).Chosen() {
							s.pushTag(full)
						}
						m.Separator()
						if m.Item("Copy Tag Name").Chosen() {
							s.copyToClipboard(full)
						}
						if m.Item("Delete Tag…").Chosen() {
							s.deleteTagConfirm(full)
						}
						if m.Item("Delete Tag from Remote…").Disabled(s.git.opBusy || len(s.git.remotes) == 0).Chosen() {
							s.confirmDeleteRemoteTag(full)
						}
					})
				})
				s.gitSidebarSection(c, "Stashes", len(s.git.stashes), &gitSectionAction{
					label: "Stash changes", menuItem: "Stash All Changes", disabled: s.git.opBusy || !s.dirtyWorktree(),
					run: s.stashChanges,
					more: func(m *ui.Menu) {
						if m.Item("Save Stash…").Disabled(s.git.opBusy || !s.dirtyWorktree()).Chosen() {
							s.openSaveStashDialog()
						}
						m.Separator()
						if m.Item("Drop All Stashes…").Disabled(s.git.opBusy || len(s.git.stashes) == 0).Chosen() {
							s.confirmDropAllStashes()
						}
					},
				}, func() {
					shown := 0
					for _, stash := range s.git.stashes {
						if !gitRefMatches(s.gitRefsFilter, stash.Ref, stash.Subject) {
							continue
						}
						shown++
						st := stash
						s.gitTreeRow(c, gitTreeItem{title: stashTitle(st.Subject), tooltip: st.Ref + "  " + st.Subject, kind: "Stash", glyph: iconStash, commit: st.Base, review: st.Hash}, func() {
							s.copyToClipboard(st.Ref)
						}, func(m *ui.Menu) {
							if m.Item("Apply Stash…").Disabled(s.git.opBusy).Chosen() {
								s.openApplyStashDialog(st)
							}
							if m.Item("Rename Stash…").Disabled(s.git.opBusy).Chosen() {
								s.openRenameStash(st)
							}
							if m.Item("Delete Stash…").Chosen() {
								s.dropStash(st.Ref)
							}
							m.Separator()
							if m.Item("Save as Patch…").Chosen() {
								s.saveStashPatch(st.Ref, st.Subject)
							}
							if m.Item("Copy Stash Ref").Chosen() {
								s.copyToClipboard(st.Ref)
							}
						})
					}
					s.gitEmptySection(c, shown)
				})
				s.submodulesSection(c)
			})
		})
	})
}

// gitSidebarSection is one collapsible group. Its title sits on the tree
// grid's title column; the creation "+" shows only under the pointer (its
// slot is always reserved, so the count never jumps) and the same action
// lives in the right-click menu.
func (s *Shell) gitSidebarSection(c *ui.Context, name string, count int, action *gitSectionAction, children func()) {
	key := s.gitFoldKey("section/" + name)
	// An empty section is its header and the 0: opening it only to read
	// "None" cost a row per Remotes/Tags/Stashes.
	closed := s.gitFolded(key) || (count == 0 && strings.TrimSpace(s.gitRefsFilter) == "")
	spec := sidebarSectionSpec{Key: "git-section:" + name, Title: name, Count: count, Closed: closed, Nav: &s.nav.gitRefs}
	if action != nil {
		if !action.disabled {
			spec.Action = &treeRowAction{Label: action.label, Tooltip: action.menuItem, Glyph: iconPlus, Run: action.run}
		}
		spec.Menu = func(m *ui.Menu) {
			if m.Item(action.menuItem).Disabled(action.disabled).Chosen() {
				action.run()
			}
			if action.more != nil {
				action.more(m)
			}
		}
	}
	if _, clicked := s.sidebarSectionHeader(c, spec); clicked {
		s.setGitFolded(key, !s.gitFolded(key))
	}
	if !closed {
		children()
	}
}

// gitWorktreeLabel is a worktree row's title: its branch, or what it lacks.
func gitWorktreeLabel(wt gitworkbench.Worktree) string {
	switch {
	case wt.Bare:
		return "(bare)"
	case wt.Branch == "":
		return "(detached)"
	}
	return wt.Branch
}
