package nativeui

/**
 * [INPUT]: sidebar.go 的提交列表（多选 ⌘ / ⇧）、gitworkbench 的 PreviewHistory / PublishedCommits / RewriteHistory / AmendTo / CherryPickCommits
 * [OUTPUT]: 提交的多选状态、右键菜单里的改写入口（Squash / Fixup / Drop / Move / Reword / Amend Staged Changes Into This Commit / Cherry-pick N）、先预览再执行的确认面板（historySheet）
 * [POS]: 2026-11-12 决定第 (12) 条：LazyGit 底座的历史改写能力接进界面。每次改写都先给用户看结果（顺序、被并入或移走的提交、已在远端的提示），确认后才跑；失败（含冲突）由内核中止并还原；整次改写可由 Undo Git Action（⌥⌘Z）退回
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"context"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

// historyLine is one row of the confirm sheet: a tag, the text, and whether
// the commit is gone afterwards (struck through).
type historyLine struct {
	tag, text string
	struck    bool
}

// historySheetState is the one confirm sheet every history rewrite goes
// through: what will happen, in order, before it does.
type historySheetState struct {
	open     bool
	title    string
	intro    string
	warn     string
	confirm  string
	lines    []historyLine
	above    int    // newer commits replayed on top, not listed
	field    string // label of the message field, "" for none
	msg      string
	run      func(ctx context.Context, root, msg string) (string, error)
	name     string // the Git operation's name in the log and the toasts
	list     ui.ListState
	needsMsg bool
}

const historyUndoHint = " If it goes wrong, Repository ▸ Undo Git Action (⌥⌘Z) puts the branch back."

// commitChosen reports whether a commit is among the ⌘ / ⇧ choice.
func (s *Shell) commitChosen(hash string) bool { return s.commitChoice[hash] }

// commitLabel names a commit as "abc1234 Subject" when the list holds it.
func (s *Shell) commitLabel(hash string) string {
	if i := slices.IndexFunc(s.git.commits, func(e gitworkbench.CommitInfo) bool { return e.Hash == hash }); i >= 0 {
		return shortHash(hash) + " " + s.git.commits[i].Subject
	}
	return shortHash(hash)
}

// commitChoiceHashes returns the chosen commits in list order (newest first).
func (s *Shell) commitChoiceHashes(entries []gitworkbench.CommitInfo) []string {
	var out []string
	for _, e := range entries {
		if s.commitChoice[e.Hash] {
			out = append(out, e.Hash)
		}
	}
	return out
}

// clearCommitChoice forgets the multi-selection.
func (s *Shell) clearCommitChoice() {
	s.commitChoice, s.commitAnchor = nil, ""
}

// pickCommit answers a click on a commit row: ⌘ toggles it among the chosen,
// ⇧ chooses the run between the anchor and it, a plain click clears the
// choice and reviews the commit. It reports whether the click was a plain one.
func (s *Shell) pickCommit(entries []gitworkbench.CommitInfo, hash string, mods ui.Modifiers) (plain bool) {
	switch {
	case mods&ui.Shift != 0 && s.commitAnchorOr(hash) != "":
		from, to := slices.IndexFunc(entries, func(e gitworkbench.CommitInfo) bool { return e.Hash == s.commitAnchorOr(hash) }),
			slices.IndexFunc(entries, func(e gitworkbench.CommitInfo) bool { return e.Hash == hash })
		if from < 0 || to < 0 {
			return true
		}
		s.commitChoice = map[string]bool{}
		for i := min(from, to); i <= max(from, to); i++ {
			s.commitChoice[entries[i].Hash] = true
		}
	case mods&ui.Cmd != 0:
		if s.commitChoice == nil {
			s.commitChoice = map[string]bool{}
			// the row the last plain click landed on (it is the one under review)
			if seed := s.commitAnchorOr(""); seed != "" {
				s.commitChoice[seed] = true
			}
		}
		if s.commitChoice[hash] {
			delete(s.commitChoice, hash)
		} else {
			s.commitChoice[hash] = true
		}
		s.commitAnchor = hash
		if len(s.commitChoice) == 0 {
			s.commitChoice = nil
		}
	default:
		s.clearCommitChoice()
		s.commitAnchor = hash
		return true
	}
	return false
}

// commitAnchorOr is the anchor of a ⇧ range: the last ⌘ / plain click, else
// the commit under review, else the clicked row itself.
func (s *Shell) commitAnchorOr(hash string) string {
	switch {
	case s.commitAnchor != "":
		return s.commitAnchor
	case s.gdReviewingCommit() && s.git.commitHash != "":
		return s.git.commitHash
	}
	return hash
}

// commitBatchMenu is the right-click menu of a choice of several commits.
func (s *Shell) commitBatchMenu(m *ui.Menu, chosen []string) {
	n := itoa(len(chosen))
	busy := s.git.opBusy
	if len(chosen) == 2 {
		// chosen is newest first: the older commit is the base of the diff.
		if m.Item("Compare the Two Commits").Chosen() {
			s.compareTwoCommits(chosen[1], chosen[0])
		}
		m.Separator()
	}
	if m.Item("Squash " + n + " Commits into the One Below…").Disabled(busy).Chosen() {
		s.openHistoryRewrite(gitworkbench.HistoryOp{Action: gitworkbench.HistorySquash, Hashes: chosen})
	}
	if m.Item("Fixup " + n + " Commits into the One Below…").Disabled(busy).Chosen() {
		s.openHistoryRewrite(gitworkbench.HistoryOp{Action: gitworkbench.HistoryFixup, Hashes: chosen})
	}
	if m.Item("Drop " + n + " Commits…").Disabled(busy).Chosen() {
		s.openHistoryRewrite(gitworkbench.HistoryOp{Action: gitworkbench.HistoryDrop, Hashes: chosen})
	}
	m.Separator()
	if m.Item("Move " + n + " Commits Up").Disabled(busy).Chosen() {
		s.openHistoryRewrite(gitworkbench.HistoryOp{Action: gitworkbench.HistoryMove, Hashes: chosen, Offset: -1})
	}
	if m.Item("Move " + n + " Commits Down").Disabled(busy).Chosen() {
		s.openHistoryRewrite(gitworkbench.HistoryOp{Action: gitworkbench.HistoryMove, Hashes: chosen, Offset: 1})
	}
	m.Separator()
	if m.Item("Cherry-pick " + n + " Commits…").Disabled(busy).Chosen() {
		s.openCherryPickMany(chosen)
	}
	m.Separator()
	if m.Item("Clear Selection").Chosen() {
		s.clearCommitChoice()
	}
}

// commitRewriteMenu is the "Rewrite History" submenu of one commit.
func (s *Shell) commitRewriteMenu(m *ui.Menu, cm gitworkbench.CommitInfo) {
	busy := s.git.opBusy
	one := []string{cm.Hash}
	m.Submenu("Rewrite History", func(sub *ui.Menu) {
		if sub.Item("Reword…").Disabled(busy).Chosen() {
			s.openReword(cm)
		}
		if sub.Item("Squash into the Commit Below…").Disabled(busy).Chosen() {
			s.openHistoryRewrite(gitworkbench.HistoryOp{Action: gitworkbench.HistorySquash, Hashes: one})
		}
		if sub.Item("Fixup into the Commit Below…").Disabled(busy).Chosen() {
			s.openHistoryRewrite(gitworkbench.HistoryOp{Action: gitworkbench.HistoryFixup, Hashes: one})
		}
		if sub.Item("Drop…").Disabled(busy).Chosen() {
			s.openHistoryRewrite(gitworkbench.HistoryOp{Action: gitworkbench.HistoryDrop, Hashes: one})
		}
		sub.Separator()
		if sub.Item("Move Up").Disabled(busy).Chosen() {
			s.openHistoryRewrite(gitworkbench.HistoryOp{Action: gitworkbench.HistoryMove, Hashes: one, Offset: -1})
		}
		if sub.Item("Move Down").Disabled(busy).Chosen() {
			s.openHistoryRewrite(gitworkbench.HistoryOp{Action: gitworkbench.HistoryMove, Hashes: one, Offset: 1})
		}
		sub.Separator()
		if sub.Item("Amend Staged Changes Into This Commit…").Disabled(busy || !s.onlyStagedChanges()).Chosen() {
			s.openAmendInto(cm)
		}
	})
}

// onlyStagedChanges is the amend-to precondition the list can see: something
// is staged and nothing else is modified (untracked files do not count).
func (s *Shell) onlyStagedChanges() bool {
	snap := s.gitSnapshot()
	if snap == nil {
		return false
	}
	staged := false
	for _, f := range snap.Files {
		if f.Staged {
			staged = true
		}
		if !f.Untracked && f.Unstaged {
			return false
		}
	}
	return staged
}

// readHistory runs a read that opens the sheet, and reports its failure as a
// toast of the operation's name.
func (s *Shell) readHistory(name string, read func(ctx context.Context, root string) (historySheetState, error)) {
	if s.git == nil || s.git.root == "" {
		return
	}
	root := s.git.root
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), gitworkbench.ReadTimeout*2)
		defer cancel()
		sheet, err := read(ctx, root)
		s.applyOnUI(func() {
			if s.git == nil || s.git.root != root {
				return
			}
			if err != nil {
				s.pendingToast = name + ": " + gitFailureText(err)
				return
			}
			sheet.open = true
			s.gitMore.history = sheet
		})
	}()
}

// publishedWarning says how many of the commits a remote already holds.
func publishedWarning(published []string) string {
	if len(published) == 0 {
		return ""
	}
	return itoa(len(published)) + " of these commits are already on a remote. Rewriting them means the branch can no longer be pushed without forcing, which Solco Git does not do yet."
}

var historyVerbs = map[gitworkbench.HistoryAction][2]string{
	gitworkbench.HistoryReword: {"Reword Commit", "Reword"},
	gitworkbench.HistorySquash: {"Squash Commits", "Squash"},
	gitworkbench.HistoryFixup:  {"Fixup Commits", "Fixup"},
	gitworkbench.HistoryDrop:   {"Drop Commits", "Drop"},
	gitworkbench.HistoryMove:   {"Move Commits", "Move"},
}

// openHistoryRewrite previews op on the loaded commits and, when it is
// possible, opens the confirm sheet. The commits are the whole list, not the
// filtered or folded view.
func (s *Shell) openHistoryRewrite(op gitworkbench.HistoryOp) {
	if s.git == nil || s.git.opBusy {
		return
	}
	verb := historyVerbs[op.Action]
	preview, err := gitworkbench.PreviewHistory(s.git.commits, op)
	if err != nil {
		s.pendingToast = verb[0] + ": " + gitFailureText(err)
		return
	}
	s.readHistory(verb[0], func(ctx context.Context, root string) (historySheetState, error) {
		published, _ := s.git.runner.PublishedCommits(ctx, root, op.Hashes)
		lines := make([]historyLine, 0, len(preview.Rows))
		for _, r := range preview.Rows {
			lines = append(lines, previewLine(r))
		}
		return historySheetState{
			title:   verb[0] + "?",
			intro:   historyIntro(op, preview),
			warn:    publishedWarning(published),
			confirm: verb[1],
			lines:   lines,
			above:   preview.Above,
			name:    verb[0],
			run: func(ctx context.Context, root, _ string) (string, error) {
				if err := s.git.runner.RewriteHistory(ctx, root, op); err != nil {
					return "", err
				}
				s.afterHistoryRewrite()
				return verb[1] + " done", nil
			},
		}, nil
	})
}

func historyIntro(op gitworkbench.HistoryOp, p gitworkbench.HistoryPreview) string {
	text := "This is how the branch looks afterwards, newest first."
	if p.Above > 0 {
		text += " The " + itoa(p.Above) + " newer commit" + pluralS(p.Above) + " above " + map[bool]string{true: "is", false: "are"}[p.Above == 1] + " replayed on top and get new hashes."
	}
	return text + " If anything conflicts, the rewrite is cancelled and nothing changes. Needs a clean working tree." + historyUndoHint
}

func previewLine(r gitworkbench.HistoryPreviewRow) historyLine {
	tag := string(r.Change)
	switch r.Change {
	case gitworkbench.ChangeSquashed, gitworkbench.ChangeFixedUp:
		tag += " → " + r.Into
	case gitworkbench.ChangeAbsorbs:
		tag = "takes " + itoa(r.Absorbed)
	}
	return historyLine{tag: tag, text: r.Short + "  " + r.Subject, struck: r.Removed}
}

// afterHistoryRewrite drops what points at commits that no longer exist: the
// choice, a commit under review, a Custom Patch selection.
func (s *Shell) afterHistoryRewrite() {
	s.applyOnUI(func() {
		if s.git == nil {
			return
		}
		s.clearCommitChoice()
		s.clearPatchSelection()
		if s.gdReviewingCommit() {
			s.reviewLocalChanges()
		}
	})
}

// openReword asks for the new message of one commit.
func (s *Shell) openReword(cm gitworkbench.CommitInfo) {
	if s.git == nil || s.git.opBusy {
		return
	}
	if _, err := gitworkbench.PreviewHistory(s.git.commits, gitworkbench.HistoryOp{Action: gitworkbench.HistoryReword, Hashes: []string{cm.Hash}, Message: "x"}); err != nil {
		s.pendingToast = "Reword Commit: " + gitFailureText(err)
		return
	}
	s.readHistory("Reword Commit", func(ctx context.Context, root string) (historySheetState, error) {
		body, err := s.git.runner.CommitMessage(ctx, root, cm.Hash)
		if err != nil {
			return historySheetState{}, err
		}
		published, _ := s.git.runner.PublishedCommits(ctx, root, []string{cm.Hash})
		return historySheetState{
			title:    "Reword Commit?",
			intro:    "Changes the message of " + cm.Short + ". The commits above it are replayed and get new hashes. If anything conflicts, nothing changes." + historyUndoHint,
			warn:     publishedWarning(published),
			confirm:  "Reword",
			field:    "New message for " + cm.Short,
			msg:      body,
			needsMsg: true,
			name:     "Reword Commit",
			lines:    []historyLine{{tag: "reword", text: cm.Short + "  " + cm.Subject}},
			run: func(ctx context.Context, root, msg string) (string, error) {
				op := gitworkbench.HistoryOp{Action: gitworkbench.HistoryReword, Hashes: []string{cm.Hash}, Message: msg}
				if err := s.git.runner.RewriteHistory(ctx, root, op); err != nil {
					return "", err
				}
				s.afterHistoryRewrite()
				return "Reworded " + cm.Short, nil
			},
		}, nil
	})
}

// openAmendInto previews folding the staged changes into an older commit.
func (s *Shell) openAmendInto(cm gitworkbench.CommitInfo) {
	if s.git == nil || s.git.opBusy || !s.onlyStagedChanges() {
		return
	}
	var files []historyLine
	for _, f := range s.gitSnapshot().Files {
		if f.Staged {
			files = append(files, historyLine{tag: "staged", text: f.Path})
		}
	}
	s.readHistory("Amend", func(ctx context.Context, root string) (historySheetState, error) {
		published, _ := s.git.runner.PublishedCommits(ctx, root, []string{cm.Hash})
		lines := append([]historyLine{{tag: "into", text: cm.Short + "  " + cm.Subject}}, files...)
		return historySheetState{
			title:   "Amend Staged Changes Into " + cm.Short + "?",
			intro:   "The staged changes become part of this commit, and the commits above it are replayed on top (new hashes). If anything conflicts, nothing changes and the changes stay staged." + historyUndoHint,
			warn:    publishedWarning(published),
			confirm: "Amend",
			lines:   lines,
			name:    "Amend",
			run: func(ctx context.Context, root, _ string) (string, error) {
				if err := s.git.runner.AmendTo(ctx, root, cm.Hash); err != nil {
					return "", err
				}
				s.afterHistoryRewrite()
				return "Amended " + cm.Short, nil
			},
		}, nil
	})
}

// openCherryPickMany previews applying the chosen commits to HEAD, oldest
// first (the order they have on their branch).
func (s *Shell) openCherryPickMany(chosen []string) {
	if s.git == nil || s.git.opBusy || len(chosen) == 0 {
		return
	}
	// The list is newest first; a cherry-pick replays oldest first.
	byHash := map[string]gitworkbench.CommitInfo{}
	for _, c := range s.git.commits {
		byHash[c.Hash] = c
	}
	var lines []historyLine
	var oldestFirst []string
	for i := len(chosen) - 1; i >= 0; i-- {
		c, ok := byHash[chosen[i]]
		if !ok {
			continue
		}
		if len(c.Parents) > 1 {
			s.pendingToast = "Cherry-pick: " + c.Short + " is a merge commit; pick its changes with Revert or Merge instead"
			return
		}
		oldestFirst = append(oldestFirst, c.Hash)
		lines = append(lines, historyLine{tag: "pick " + itoa(len(oldestFirst)), text: c.Short + "  " + c.Subject})
	}
	if len(oldestFirst) == 0 {
		return
	}
	s.gitMore.history = historySheetState{
		open:    true,
		title:   "Cherry-pick " + itoa(len(oldestFirst)) + " Commits?",
		intro:   "Applies them to the current branch in this order. Needs a clean working tree. If one conflicts, the pick stops there and the merge banner offers Continue, Skip and Abort.",
		confirm: "Cherry-pick",
		lines:   lines,
		name:    "Cherry-pick Commits",
		run: func(ctx context.Context, root, _ string) (string, error) {
			if err := s.git.runner.CherryPickCommits(ctx, root, oldestFirst); err != nil {
				return "", err
			}
			s.applyOnUI(s.clearCommitChoice)
			return "Cherry-picked " + itoa(len(oldestFirst)) + " commits", nil
		},
	}
}

func (s *Shell) historySheet(c *ui.Context) {
	h := &s.gitMore.history
	if !h.open {
		return
	}
	t := c.Theme()
	ui.Modal(c, &h.open, func() {
		ui.Column(c).Width(660).MaxWidth(820).Gap(Spacing().M).Children(func() {
			modalTitle(c, h.title)
			ui.Text(c, h.intro).FontSize(Typography().BodySmall).TextColor(t.TextMuted).MaxWidth(620)
			if h.warn != "" {
				ui.Text(c, h.warn).FontSize(Typography().BodySmall).TextColor(t.Warning).MaxWidth(620)
			}
			if h.above > 0 {
				quietNote(c, "… "+itoa(h.above)+" newer commit"+pluralS(h.above)+", replayed on top")
			}
			ui.List(c, &h.list, len(h.lines), func(i int) {
				if i < 0 || i >= len(h.lines) {
					return
				}
				historyLineView(c, h.lines[i])
			}).MaxHeight(300).MinHeight(0)
			if h.needsMsg {
				textArea(c, &h.msg).FillWidth().Height(96).Label(h.field)
			}
			ok := !h.needsMsg || strings.TrimSpace(h.msg) != ""
			ui.Row(c).FillWidth().Gap(Spacing().M).Justify(ui.End).Children(func() {
				if regularButton(c, "Cancel").Clicked() {
					h.open = false
				}
				if regularPrimaryButton(c, h.confirm).Disabled(!ok || s.git.opBusy).Clicked() {
					run, msg, name := h.run, h.msg, h.name
					h.open = false
					s.runGitOp(name, func(ctx context.Context, root string) (string, error) { return run(ctx, root, msg) })
				}
			})
		})
	})
}

func historyLineView(c *ui.Context, l historyLine) {
	t := c.Theme()
	row := ui.Row(c).Gap(8).Padding(3, 8).AlignItems(ui.Center).MinWidth(0)
	row.Children(func() {
		tone := t.TextMuted
		if l.tag != "" {
			ui.Row(c).Size(118, 18).Shrink(0).Center().Radius(treeRowRadius).Background(tone.Alpha(0.14)).Children(func() {
				ui.Text(c, l.tag).FontSize(Typography().Caption).FontWeight(WeightMedium).TextColor(tone).SingleLine()
			})
		} else {
			ui.Box(c).Size(118, 18).Shrink(0)
		}
		text := ui.Text(c, l.text).Font(gdCodeFont()).FontSize(Typography().BodySmall).Grow(1).MinWidth(0).SingleLine()
		if l.struck {
			text.Strikethrough().TextColor(t.TextMuted)
		}
	})
}
