package nativeui

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

// gitCleanState is the center of the Changes view on a clean worktree: not a
// page of its own but the empty-diff banner every other empty review uses
// (2026-11-16), saying so and where the branch stands against its upstream,
// with the one action that follows (Push when ahead, Pull when behind, Merge
// when diverged, Publish without an upstream, else Fetch). The file list
// column beside it keeps its place; the latest commits are the Commits view's.
func (s *Shell) gitCleanState(c *ui.Context, title string) {
	where := s.currentBranchName()
	if where == "" {
		where = gdAbbreviateHome(s.git.root)
	}
	s.gdEmptyPanel(c, nil, title, upstreamSummary(where, s.git.upstream), s.cleanStateAction(c))
}

// cleanStateAction draws the clean banner's one button; nil on a detached HEAD,
// where no branch has an upstream to speak of.
func (s *Shell) cleanStateAction(c *ui.Context) func() {
	branch := s.currentBranchName()
	if branch == "" {
		return nil
	}
	st := s.git.upstream
	busy := s.git.opBusy || s.git.sequencerChecked && s.git.sequencer.Active()
	return func() {
		switch {
		case st.Diverged():
			// Push is refused for sure and a fast-forward pull too: the one
			// way forward is to bring the remote's work in.
			if compactPrimaryButton(c, "Merge "+st.Remote+"/"+st.Branch+"…").Disabled(busy).Clicked() {
				s.openMergeDialog(st.Remote + "/" + st.Branch)
			}
		case !st.OK:
			if compactPrimaryButton(c, "Publish Branch").Disabled(busy).Clicked() {
				s.pushRepo()
			}
		case st.Ahead > 0:
			if compactPrimaryButton(c, "Push "+itoa(st.Ahead)+" commit"+pluralS(st.Ahead)).Disabled(busy).Clicked() {
				s.pushRepo()
			}
		case st.Behind > 0:
			if compactPrimaryButton(c, "Pull "+itoa(st.Behind)+" commit"+pluralS(st.Behind)).Disabled(busy).Clicked() {
				s.pullRepo()
			}
		default:
			if compactButton(c, "Fetch").Disabled(busy).Clicked() {
				s.fetchRepo()
			}
		}
	}
}

// upstreamSummary says where a branch stands against its upstream in one
// line: ahead, behind, level, or without one.
func upstreamSummary(where string, st gitworkbench.UpstreamStatus) string {
	var parts []string
	if where != "" {
		parts = append(parts, where)
	}
	switch {
	case !st.OK:
		parts = append(parts, "No upstream")
	case st.Ahead == 0 && st.Behind == 0:
		parts = append(parts, "Up to date with "+st.Remote+"/"+st.Branch)
	default:
		if st.Ahead > 0 {
			parts = append(parts, "↑"+itoa(st.Ahead)+" ahead of "+st.Remote+"/"+st.Branch)
		}
		if st.Behind > 0 {
			parts = append(parts, "↓"+itoa(st.Behind)+" behind")
		}
		if st.Diverged() {
			parts = append(parts, "Diverged — merge or rebase before pushing")
		}
	}
	return strings.Join(parts, " · ")
}
