package nativeui

import (
	"context"
	"slices"
	"time"

	"github.com/thewei/solco-gitkit/gitworkbench"
)

// ensureGitSnapshot applies a cached snapshot or starts a background refresh.
// Uses the dedicated snapshot generation: branch loading, file expansion or
// terminal find can never drop a snapshot result (P0-04).
func (s *Shell) ensureGitSnapshot(force bool) {
	if s.git == nil || s.git.root == "" {
		return
	}
	root := s.git.root
	if !force {
		if snap, fresh := s.git.cache.Get(root); fresh {
			s.applyGitSnapshot(snap)
			return
		}
		if !s.git.cache.NeedsRefresh(root) {
			return
		}
	}
	if s.git.refreshing {
		return
	}
	s.git.refreshing = true
	gen := s.git.snapshotGen.Add(1)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*gitworkbench.ReadTimeout)
		defer cancel()
		snap, _ := s.git.cache.Refresh(ctx, root)
		s.applyGuarded(s.git.snapshotGen.Load, gen, func() {
			s.git.refreshing = false
			if snap != nil && snap.Root == s.git.root {
				s.applyGitSnapshot(snap)
			}
		})
	}()
}

// applyGitSnapshot swaps the visible change snapshot when the context still
// matches (stale results may populate the cache but never the view, plan §36).
// The upper Changes file tree is invalidated for the next projection.
// A signature that differs from the displayed one raises nothing: every
// view that shows the changes re-reads them itself — on a tick while on
// screen, on window activation, after the app's own operations — so the
// fresh data is the notification. A "changed on disk, refresh to update"
// pill here could only be a false alarm over data already updated
// (2026-11-08: it appeared after the first load and after the user's own
// operations).
func (s *Shell) applyGitSnapshot(snap *gitworkbench.ChangesSnapshot) {
	if snap == nil || snap.Root != s.git.root {
		return
	}
	s.git.snapshot = snap
	s.changesTree = nil
	s.gdSetFiles()

	// Selected file removed on refresh falls back to a valid one (GWB-183).
	if s.surface.diff.SelectedPath != "" {
		if _, ok := snap.ByPath(s.surface.diff.SelectedPath); !ok {
			s.surface.diff.SelectedPath = firstChangedPath(snap)
		}
	}
	s.rebuildDiffRows()
	if p := s.git.pendingReveal; p != "" {
		s.git.pendingReveal = ""
		if p == s.surface.diff.SelectedPath { // the file is changed: the pick stood
			s.revealDiffFile(p)
		}
	}
}

func firstChangedPath(snap *gitworkbench.ChangesSnapshot) string {
	// The header Changes toggle calls openChanges("") before any snapshot
	// exists (non-repo cwd, refresh still in flight): that must degrade to
	// an empty Diff selection, not crash the process.
	if snap == nil {
		return ""
	}
	for _, cf := range snap.Files {
		return cf.Path
	}
	return ""
}

// refreshGitChanges is the explicit Refresh action.
func (s *Shell) refreshGitChanges() {
	if s.git == nil || s.git.root == "" {
		return
	}
	s.git.cache.Invalidate(s.git.root)
	s.ensureGitSnapshot(true)
	s.loadBranches()
}

// focusRefreshGap is the smallest gap between two activation refreshes.
// Flyouts and dialogs hand the key window back and forth; each hand-back
// fires OnFocus, and only a real absence of the app deserves a sync.
const focusRefreshGap = 2 * time.Second

// refreshGitOnActivation re-syncs the visible changes snapshot when the
// window gains focus: work done outside the app must show without a
// manual refresh, which stays as the explicit fallback — the fresh data is
// the notification.
func (s *Shell) refreshGitOnActivation() {
	if s.git == nil || s.git.root == "" {
		return
	}
	now := timeNow()
	if !s.git.lastActivationRefresh.IsZero() && now.Sub(s.git.lastActivationRefresh) < focusRefreshGap {
		return
	}
	s.git.lastActivationRefresh = now
	s.git.cache.Invalidate(s.git.root)
	s.ensureGitSnapshot(true)
	s.loadSequencer()
}

// loadSequencer refreshes only the merge/revert/cherry-pick probe (one
// bounded read of .git sequencer files plus the unmerged count). The
// activation sync runs it so a sequencer state created outside Solco shows
// its banner without a manual refresh; the explicit Refresh and the
// post-mutation path probe it through loadBranches instead.
func (s *Shell) loadSequencer() {
	if s.git == nil || s.git.root == "" {
		return
	}
	root := s.git.root
	gen := s.git.sequencerGen.Add(1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), gitworkbench.ReadTimeout)
		defer cancel()
		sequencer, err := s.git.runner.SequencerState(ctx, root)
		s.applyGuarded(s.git.sequencerGen.Load, gen, func() {
			if s.git == nil || root != s.git.root {
				return
			}
			s.git.sequencer = sequencer
			s.git.sequencerChecked = err == nil
		})
	}()
}

// loadBranches refreshes the local branch list, the upstream tracking
// status and the stash list in one background lane. Its own generation keeps
// it independent of snapshot/commit/terminal-find work (P0-04).
func (s *Shell) loadBranches() {
	if s.git == nil || s.git.root == "" {
		return
	}
	root := s.git.root
	s.ensureAutoFetch()
	s.git.branchesLoading = true
	s.git.commitsLoading = true
	gen := s.git.branchesGen.Add(1)
	graphGen := s.git.graphGen.Load()
	graphQuery := gitworkbench.GraphQuery{Limit: s.graphWindow(), Hidden: slices.Clone(s.graphHiddenList())}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), gitworkbench.ReadTimeout)
		defer cancel()
		branches, err := s.git.runner.ListBranches(ctx, root)
		remoteBranches, _ := s.git.runner.ListRemoteBranches(ctx, root)
		upstream, _ := s.git.runner.Upstream(ctx, root)
		stashes, _ := s.git.runner.StashList(ctx, root)
		tags, _ := s.git.runner.ListTags(ctx, root)
		worktrees, _ := s.git.runner.ListWorktrees(ctx, root)
		remotes, _ := s.git.runner.ListRemotes(ctx, root)
		submodules, _ := s.git.runner.ListSubmodules(ctx, root)
		commits, _ := s.git.runner.ListCommitGraph(ctx, root, graphQuery)
		sequencer, sequencerErr := s.git.runner.SequencerState(ctx, root)
		s.applyGuarded(s.git.branchesGen.Load, gen, func() {
			s.git.branchesLoading = false
			s.git.commitsLoading = false
			if root == s.git.root {
				s.git.refsRead = true
			}
			if err != nil || root != s.git.root {
				return
			}
			s.git.branches = branches
			s.git.remoteBranches = remoteBranches
			s.git.upstream = upstream
			s.git.stashes = stashes
			s.git.tags = tags
			s.git.worktrees = worktrees
			s.git.remotes, s.git.submodules = remotes, submodules
			// A graph re-read (eye, widened window) that started after this
			// one began owns the list.
			if s.git.graphGen.Load() == graphGen {
				s.git.commits = commits
			} else {
				commits = s.git.commits
			}
			// A history view opened before refs finished loading gets its
			// first read-only commit after the root/generation fence.
			if s.commitsMode && !s.gdReviewingCommit() && len(commits) > 0 {
				s.reviewCommit(commits[0].Hash)
			}
			s.git.sequencer = sequencer
			s.git.sequencerChecked = sequencerErr == nil
		})
	}()
}

// currentBranchName returns the branch reported by the current snapshot.
func (s *Shell) currentBranchName() string {
	if s.git == nil || s.git.snapshot == nil {
		return ""
	}
	return s.git.snapshot.Branch
}

// openChanges opens the Changes-driven Diff surface (changed-file click).
func (s *Shell) openChanges(path string) {
	ctx := s.workspaceContext()
	if path == "" {
		path = firstChangedPath(s.gitSnapshot())
	}
	s.surface.openDiff(ctx, path)
	if s.git != nil {
		gitworkbench.EnsureAncestorsOpen(s.git.collapsedDirs, path)
	}
	s.ensureGitSnapshot(false)
	s.rebuildDiffRows()
}
