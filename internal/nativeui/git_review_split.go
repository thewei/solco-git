package nativeui

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

const (
	gitSplitMinTop          float32 = 170
	gitSplitMinBottom       float32 = 220
	gitSplitDefaultTop      float32 = 420
	gitFilePaneDefaultWidth float32 = 292
	gitFilePaneMinWidth     float32 = 205
	gitFileSplitterWidth    float32 = splitHitWidth
	// gitFilePaneCollapseBelow is the width of the center (the window less
	// the sidebar) under which the file navigator closes by itself: open at
	// its default width, it would leave the diff under ~450 px.
	gitFilePaneCollapseBelow float32 = 750
)

// gitSplitReviewSurface intentionally has two different spatial hierarchies:
// Changes: file navigator | (diff + compact commit composer)
// Commits: commit history above (commit message + changed files | read-only diff).
func (s *Shell) gitSplitReviewSurface(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
		if s.commitsMode {
			_, windowH := c.Size()
			maxTop := windowH - gitSplitMinBottom - 85
			if maxTop < gitSplitMinTop {
				maxTop = gitSplitMinTop
			}
			if s.gitUpperHeight == 0 {
				s.gitUpperHeight = gitSplitDefaultTop
			}
			upper := clampF(s.gitUpperHeight, gitSplitMinTop, maxTop)
			ui.Column(c).FillWidth().Height(upper).Shrink(0).MinHeight(0).Children(func() {
				s.gitCommitHistoryPane(c)
			})
			if drag := splitHandle(c, s.tokens(t.Dark), splitHorizontal, "Resize Git history and diff panes"); drag.Reset {
				s.gitUpperHeight = clampF(gitSplitDefaultTop, gitSplitMinTop, maxTop)
			} else if drag.Held {
				s.gitUpperHeight = clampF(upper+drag.DY, gitSplitMinTop, maxTop)
			}
			s.gitCommitReview(c)
			return
		}
		s.gitLocalChangesReview(c)
	})
}

func (s *Shell) gitLocalChangesReview(c *ui.Context) {
	ui.Row(c).Grow(1).MinHeight(0).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
		// A clean worktree has no files to navigate: the column would be a
		// "0 files" strip beside the status page.
		if snap := s.gitSnapshot(); snap == nil || len(snap.Files) > 0 {
			s.gitFileNavigationColumn(c, func() { s.sidebarFilesView(c) })
		}
		ui.Column(c).Grow(1).MinHeight(0).MinWidth(0).Children(func() {
			ui.Column(c).Grow(1).MinHeight(0).MinWidth(0).Children(func() {
				s.gdDiffSurface(c)
			})
			s.gitQuickCommitComposer(c)
		})
	})
}

// gitFilePaneLimits is the navigator's width in a window this wide, and the
// widest it may be: the user's drag, kept clear of the sidebar and the diff.
func (s *Shell) gitFilePaneLimits(windowW float32) (width, maxWidth float32) {
	width = s.gitFilePaneWidth
	if width == 0 {
		width = gitFilePaneDefaultWidth
	}
	maxWidth = windowW - s.sidebarW - 270
	if maxWidth < gitFilePaneMinWidth {
		maxWidth = gitFilePaneMinWidth
	}
	return clampF(width, gitFilePaneMinWidth, maxWidth), maxWidth
}

// gitCenterWidth is the room the Git surface shares between its file
// navigator and its diff: the window less the sidebar, when it shows.
func (s *Shell) gitCenterWidth(windowW float32) float32 {
	if s.sidebarCollapsed {
		return windowW
	}
	return windowW - s.sidebarW
}

// gitFilePaneOpen tells whether the file navigator shows in a window this
// wide. A narrow center closes it by itself, to give the diff its width. The
// user's say is kept per width class: they open it in a narrow window, and
// close it in a wide one.
func (s *Shell) gitFilePaneOpen(windowW float32) bool {
	if s.gitCenterWidth(windowW) < gitFilePaneCollapseBelow {
		return s.gitFilePaneOpenNarrow
	}
	return !s.gitFilePaneClosedWide
}

// toggleGitFilePane opens or closes the navigator in the width class the
// window is in.
func (s *Shell) toggleGitFilePane(windowW float32) {
	if s.gitCenterWidth(windowW) < gitFilePaneCollapseBelow {
		s.gitFilePaneOpenNarrow = !s.gitFilePaneOpenNarrow
	} else {
		s.gitFilePaneClosedWide = !s.gitFilePaneClosedWide
	}
}

// gitFilePaneToggle is the toolbar button that shows or hides the navigator.
func (s *Shell) gitFilePaneToggle(c *ui.Context) {
	windowW, _ := c.Size()
	label := "Show file list"
	if s.gitFilePaneOpen(windowW) {
		label = "Hide file list"
	}
	if gdIconButton(c, iconPanel, label).Clicked() {
		s.toggleGitFilePane(windowW)
	}
}

// gitFileNavigationColumn is the reusable resize seam for the middle-left
// file tree. It does not reuse (or compete with) the app's Git reference rail.
// Closed, it leaves the whole width to the diff beside it.
func (s *Shell) gitFileNavigationColumn(c *ui.Context, body func()) {
	t := c.Theme()
	windowW, _ := c.Size()
	if !s.gitFilePaneOpen(windowW) {
		return
	}
	width, maxWidth := s.gitFilePaneLimits(windowW)
	ui.Column(c).Width(width).FillHeight().Shrink(0).MinHeight(0).Children(body)
	if drag := splitHandle(c, s.tokens(t.Dark), splitVertical, "Resize Git file navigator"); drag.Reset {
		s.gitFilePaneWidth = clampF(gitFilePaneDefaultWidth, gitFilePaneMinWidth, maxWidth)
	} else if drag.Held {
		s.gitFilePaneWidth = clampF(width+drag.DX, gitFilePaneMinWidth, maxWidth)
	}
}

// Quick composer is an entry point into the SINGLE existing fenced Commit
// modal. It never calls git itself or duplicates the commit transaction.
func (s *Shell) gitQuickCommitComposer(c *ui.Context) {
	t := c.Theme()
	snap := s.gitSnapshot()
	if snap == nil || s.gdReviewingCommit() {
		return
	}
	// A clean tree has nothing to commit: the bar goes unless a draft or an
	// amend is in progress (2026-10-30 UX audit — the empty state is one block).
	if len(snap.Files) == 0 && s.gitQuickSubject == "" && s.gitQuickBody == "" && !s.gitQuickAmend {
		return
	}
	// A merge/rebase/cherry-pick in progress owns the next step (the banner's
	// Continue/Abort): a disabled Commit bar beneath it only invites a typed
	// message that nothing can send (device E2E 2026-11-03).
	if s.git.sequencerChecked && s.git.sequencer.Active() {
		return
	}
	ui.Column(c).FillWidth().Shrink(0).Padding(8, 12).
		BorderWidth(1, 0, 0, 0).BorderColor(s.tokens(t.Dark).BorderSubtle).Children(func() {
		contentColumn(c, formColumnWidth, func() {
			s.gitQuickCommitForm(c, snap)
		})
	})
}

// gitQuickCommitForm is the composer's fields and actions; the caller bounds
// its width so the inputs never run across a wide diff. Idle it is one line —
// the subject and the Commit action; focusing it (or a draft, an amend, a
// running draft) opens the description and the options row.
func (s *Shell) gitQuickCommitForm(c *ui.Context, snap *gitworkbench.ChangesSnapshot) {
	t := c.Theme()
	blocked := s.git.opBusy || s.git.committing || s.git.branchBusy ||
		len(snap.Files) == 0 || (s.git.sequencerChecked && s.git.sequencer.Active())
	form := ui.Column(c).FillWidth().Gap(5)
	open := s.gitQuickSubject != "" || s.gitQuickBody != "" || s.gitQuickAmend || form.FocusWithin()
	form.Children(func() {
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			textField(c.Key("git-quick-subject"), &s.gitQuickSubject).Grow(1).MinWidth(0).Height(Controls().RegularHeight).
				Placeholder("Commit subject").Label("Commit subject draft")
			if !open {
				s.gitQuickCommitActions(c, blocked)
			}
		})
		if !open {
			return
		}
		textField(c.Key("git-quick-body"), &s.gitQuickBody).FillWidth().Height(Controls().RegularHeight).
			Placeholder("Description (optional)").Label("Commit description draft")
		ui.Row(c).FillWidth().Height(28).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Checkbox(c, &s.gitQuickAmend, "Amend")
			ui.Spacer(c)
			ui.Text(c, pluralFiles(len(snap.Files))).FontSize(Typography().BodySmall).TextColor(t.TextMuted)
			s.gitQuickCommitActions(c, blocked)
		})
	})
}

// gitQuickCommitActions is the Commit button and its options pull-down.
func (s *Shell) gitQuickCommitActions(c *ui.Context, blocked bool) {
	t := c.Theme()
	// KeepFocus: a press must not move the focus onto the button. Focus in the
	// form opens it and moves the buttons to the options row mid-click, and the
	// release then lands on another button — the first click only opened the
	// form (2026-11-08 run). Tab still reaches the buttons.
	button := compactPrimaryButton(c, "Commit").KeepFocus()
	button.Disabled(blocked)
	if button.Clicked() {
		s.openQuickCommit(false)
	}
	dropdown := gorexIconButton(c, gorexColorsOf(t.Dark), iconChevronDown, "Commit options", "Commit options",
		false, gorexPaneBtn, gorexPaneGlyph).KeepFocus()
	dropdown.Menu(func(m *ui.Menu) {
		if m.Item("Commit and Push…").Disabled(blocked).Chosen() {
			s.openQuickCommit(true)
		}
	})
}

// openQuickCommit opens the Commit dialog with the composer's draft; push
// makes its success continue with a Push (Commit and Push…).
func (s *Shell) openQuickCommit(push bool) {
	s.openCommitSurface()
	s.surface.commit.Subject = s.gitQuickSubject
	s.surface.commit.Body = s.gitQuickBody
	s.surface.commit.Amend = s.gitQuickAmend
	// Do not overwrite a user-written amend message with HEAD.
	s.surface.commit.AmendPrefilled = s.gitQuickAmend &&
		(s.gitQuickSubject != "" || s.gitQuickBody != "")
	s.gitMore.pushAfterCommit = push
}

// showGitCommits switches both the upper navigator and lower reviewer.
func (s *Shell) showGitCommits() {
	s.commitsMode = true
	if s.git != nil && !s.gdReviewingCommit() && len(s.git.commits) > 0 {
		s.reviewCommit(s.git.commits[0].Hash)
	}
}

// gitCommitHistoryPane is the wide top table of the Commits view: a one-row
// head (search button + count) over the graph list.
func (s *Shell) gitCommitHistoryPane(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		s.gitListHead(c, &headerSearch{
			Query: &s.gitCommitFilter, Open: &s.gitCommitSearchOpen, Label: "Search commits", Button: "Search commit list",
		}, 12, func() {
			s.commitFilterButton(c)
			summary := plural(len(s.git.commits), "commit", "commits")
			switch {
			case s.commitFilterOn():
				summary = plural(len(s.filteredCommits()), "commit", "commits") + " · " + s.commitFilterSummary()
				if s.gitMore.cfilter.loading {
					summary = "Filtering…"
				}
			case strings.TrimSpace(s.gitCommitFilter) != "":
				summary = itoa(len(s.filteredCommits())) + " of " + summary
			}
			ui.Text(c, summary).FontSize(Typography().BodySmall).TextColor(t.TextMuted).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
			if s.commitFilterOn() && sectionTextAction(c, "Clear Filters", "Show every commit again").Clicked() {
				s.clearCommitFilter()
			}
			if n := s.graphHiddenCount(); n > 0 {
				if sectionTextAction(c, itoa(n)+" hidden · Show All", "Show every branch in the graph").Clicked() {
					s.showAllInGraph()
				}
			}
		})
		s.gitCommitColumnHead(c)
		s.commitsPane(c, gdPaletteFor(t))
	})
}

// gitCommitReview is the lower half of the Commits view: the commit's message
// and changed files on the left, its diff on the right. A narrow center closes
// the left column; the toolbar button above the diff brings it back.
func (s *Shell) gitCommitReview(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).MinHeight(0).MinWidth(0).Children(func() {
		if !s.gdReviewingCommit() {
			msg := "Select a commit to review its changes"
			if !s.git.commitsLoading && len(s.git.commits) == 0 {
				msg = "This repository has no commits yet"
			}
			s.gdEmptyPanel(c, gdPaletteFor(t), "No commit selected", msg, nil)
			return
		}
		s.gitCommitStrip(c)
		ui.Row(c).Grow(1).MinHeight(0).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
			s.gitFileNavigationColumn(c, func() {
				if s.git.commitMeta != nil {
					s.commitTreeSync(s.git.commitMeta.Hash)
				}
				if !s.git.compare {
					s.commitScopeTabs(c)
				}
				if s.treeAt() {
					s.commitTreeList(c)
					return
				}
				s.gitCommitFiles(c)
			})
			ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
				if s.treeAt() {
					ui.Row(c).FillWidth().Padding(6, 12, 0).Gap(Spacing().S).AlignItems(ui.Center).Children(func() {
						s.gitFilePaneToggle(c)
					})
					s.commitTreeFileView(c)
					return
				}
				ui.Row(c).FillWidth().Padding(6, 12, 0).Gap(Spacing().S).AlignItems(ui.Center).Children(func() {
					s.gitFilePaneToggle(c)
					ui.Spacer(c)
					s.gdLayoutControl(c)
				})
				s.gdDiffSurface(c)
			})
		})
	})
}

func (s *Shell) gitCommitFiles(c *ui.Context) {
	t := c.Theme()
	snap := s.git.commitSnap
	ui.Column(c).Grow(1).MinHeight(0).MinWidth(0).Children(func() {
		s.gitListHead(c, &headerSearch{
			Query: &s.gitCommitFilesFilter, Open: &s.gitCommitFilesSearchOpen, Label: "Filter commit files", Button: "Search commit files",
		}, 10, func() {
			ui.Text(c, "Changed Files").FontSize(Typography().BodySmall).FontWeight(WeightSemibold).SingleLine()
			if snap != nil {
				ui.Textf(c, "%d", len(snap.Files)).FontSize(Typography().BodySmall).TextColor(t.TextMuted)
			}
		})
		if snap == nil {
			ui.Column(c).Grow(1).Center().Children(func() { ui.Spinner(c).Size(15, 15) })
			return
		}
		rows := gitworkbench.Flatten(gitworkbench.FilterTree(
			gitworkbench.BuildTree(snap.Files, true), s.gitCommitFilesFilter), s.git.collapsedDirs)
		list := s.gdChangedTreeList(c, gdPaletteFor(t), rows, &s.changesList, &s.changesSelected,
			nil, gdViewCombined, func(row gitworkbench.FlatRow) {
				if row.Node.Dir {
					s.toggleChangesDir(row.Node.Path)
				} else {
					s.revealDiffFile(row.Node.Path)
				}
			})
		list.Children(func() {
			if len(rows) == 0 {
				quietNote(c, "No matching files")
			}
		})
	})
}

// gitCommitBodyFold is the body's height, in lines, while it is folded.
const gitCommitBodyFold = 3

// splitCommitMessage cuts a full message into its subject and body; the
// subject falls back to the list's when the message could not be read.
func splitCommitMessage(message, subject string) (string, string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return subject, ""
	}
	head, body, _ := strings.Cut(message, "\n")
	return strings.TrimSpace(head), strings.TrimSpace(body)
}

// gitCommitStrip is the commit under review, one header across the whole
// review (Fork's layout, 2026-11-03: it used to be a card squeezed into the
// file column above the file list): the author's avatar and name, the short
// hash, the date and the subject on one line, with the commit's actions
// behind one menu. A message body sits on a line of its own beneath, folded
// past three lines.
func (s *Shell) gitCommitStrip(c *ui.Context) {
	t := c.Theme()
	cm := s.git.commitMeta
	ui.Column(c).FillWidth().Shrink(0).Padding(8, 12).Gap(6).
		BorderWidth(0, 0, 1, 0).BorderColor(s.tokens(t.Dark).BorderSubtle).Children(func() {
		if cm == nil {
			busyNote(c, "Reading commit…")
			return
		}
		if s.git.compare && s.git.pairBase != "" {
			base := s.git.pairBase
			ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Compared "+shortHash(base)+" → "+cm.Short).FontSize(Typography().Body).FontWeight(WeightSemibold).Shrink(0).SingleLine()
				tipped(c, ui.Text(c, "What changes from "+s.commitLabel(base)+" to "+s.commitLabel(cm.Hash)+".").
					FontSize(Typography().BodySmall).TextColor(t.TextMuted).Grow(1).MinWidth(0).SingleLine(), cm.Subject)
				if compactButton(c, "Back to Commit").Clicked() {
					s.reviewCommitAs(cm.Hash, false)
				}
			})
			return
		}
		if s.git.compare {
			ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Working tree compared with "+cm.Short).FontSize(Typography().Body).FontWeight(WeightSemibold).Shrink(0).SingleLine()
				tipped(c, ui.Text(c, "What changed between that commit and the files on disk now — staged and unstaged, untracked files not included.").
					FontSize(Typography().BodySmall).TextColor(t.TextMuted).Grow(1).MinWidth(0).SingleLine(), cm.Subject)
				if compactButton(c, "Back to Commit").Clicked() {
					s.reviewCommitAs(cm.Hash, false)
				}
			})
			return
		}
		subject, body := splitCommitMessage(s.git.commitBody, cm.Subject)
		message := subject
		if body != "" {
			message += "\n\n" + body
		}
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
			authorAvatar(c, cm.Author, 20)
			ui.Text(c, cm.Author).FontSize(Typography().Body).FontWeight(WeightSemibold).Shrink(0).SingleLine()
			ui.Text(c, cm.Short).Font(gdCodeFont()).FontSize(Typography().BodySmall).TextColor(t.TextMuted).Shrink(0).Selectable()
			ui.Text(c, gdCommitTime(cm.Time)).FontSize(Typography().BodySmall).TextColor(t.TextMuted).Shrink(0).SingleLine()
			ui.Text(c, subject).FontSize(Typography().Body).FontWeight(WeightSemibold).Grow(1).MinWidth(0).SingleLine().Selectable()
			menu := gorexIconButton(c, gorexColorsOf(t.Dark), iconActions, "Commit actions", "Commit actions", false, gorexRowBtn, gorexRowGlyph)
			menu.Menu(func(m *ui.Menu) {
				if m.Item("Copy Commit Hash").Chosen() {
					s.copyToClipboard(cm.Hash)
				}
				if m.Item("Copy Message").Chosen() {
					s.copyToClipboard(message)
					s.pendingToast = "Copied commit message"
				}
				m.Separator()
				if m.Item("Cherry-pick…").Chosen() {
					s.confirmCherryPickCommit(cm.Hash, cm.Subject)
				}
				if m.Item("Revert…").Chosen() {
					s.confirmRevertCommit(cm.Hash, cm.Subject)
				}
			})
		})
		if body == "" {
			return
		}
		ui.Column(c).FillWidth().Gap(2).Children(func() {
			text := ui.Text(c, body).FontSize(Typography().BodySmall).TextColor(t.TextMuted).Selectable()
			if !s.gitCommitBodyOpen {
				text.MaxLines(gitCommitBodyFold)
			}
			if strings.Count(body, "\n") >= gitCommitBodyFold || len(body) > 180 {
				label := "Show more"
				if s.gitCommitBodyOpen {
					label = "Show less"
				}
				fold := ui.ButtonBase(c).Padding(0).FocusRing(false).Label(label).Children(func() {
					ui.Text(c, label).FontSize(Typography().Caption).TextColor(t.Accent)
				})
				if fold.Clicked() {
					s.gitCommitBodyOpen = !s.gitCommitBodyOpen
				}
			}
		})
	})
}
