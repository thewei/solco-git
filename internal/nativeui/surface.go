package nativeui

import (
	"sync"

	"github.com/thewei/solco-git/internal/settings"
)

// WorkspaceSurfaceKind is the center surface of the Changes / Commits view:
// the Diff review, or the Commit editor over it. (Solco's Terminal and Chat
// surfaces do not exist here.)
type WorkspaceSurfaceKind string

const (
	WorkspaceSurfaceDiff   WorkspaceSurfaceKind = "diff"
	WorkspaceSurfaceCommit WorkspaceSurfaceKind = "commit"
)

// WorkspaceContextKey identifies the repository a surface state belongs to.
// RepoRoot is the resolved Git repository root of the workspace ("" when its
// directory is not inside a repository).
type WorkspaceContextKey struct {
	RepoRoot string
}

// DiffSurfaceState is the Diff surface's own presentation state.
type DiffSurfaceState struct {
	SelectedPath string
	SplitLayout  bool
}

// CommitSurfaceState is the Commit surface's own presentation state.
type CommitSurfaceState struct {
	Subject       string
	Body          string
	Selected      map[string]bool
	Captured      bool // stale-state fence captured at open
	InFlight      bool
	ErrText       string
	CommittedHash string
	// Amend replaces the last commit; AmendPrefilled guards loading HEAD's
	// message into the fields once per toggle.
	Amend          bool
	AmendPrefilled bool
	// NoVerify skips the repository's commit hooks; SignOff adds a
	// Signed-off-by trailer. Both apply to this one commit only.
	NoVerify bool
	SignOff  bool
	// CoAuthors is "Name <email>" people separated by semicolons (Co-authored-by
	// trailers); ResetAuthor, with Amend, makes the amended commit mine.
	CoAuthors   string
	ResetAuthor bool
}

// workspaceSurfaceState is the UI-independent primary-surface state machine
// for one /workspace center (plan §6 transition table). Pure state: no Herdr
// objects, no Git calls, no IO — transition tests run headless.
type workspaceSurfaceState struct {
	mu sync.Mutex

	kind    WorkspaceSurfaceKind
	context WorkspaceContextKey
	diff    DiffSurfaceState
	commit  CommitSurfaceState
	// previous is the surface restored when a Commit cancels.
	previous WorkspaceSurfaceKind
}

// start puts the machine on the Diff review, where every workspace begins.
func (w *workspaceSurfaceState) start(ctx WorkspaceContextKey) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.kind, w.previous, w.context = WorkspaceSurfaceDiff, WorkspaceSurfaceDiff, ctx
}

func (w *workspaceSurfaceState) current() WorkspaceSurfaceKind {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.kind
}

// openDiff implements "changed file click" / "Changes open": switch to the
// Diff surface for path ("" = current/first selection).
func (w *workspaceSurfaceState) openDiff(ctx WorkspaceContextKey, path string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.context != ctx {
		w.diff = DiffSurfaceState{}
	}
	w.context = ctx
	if path != "" || w.diff.SelectedPath == "" {
		w.diff.SelectedPath = path
	}
	w.previous = w.previousSurfaceFor(w.kind)
	w.kind = WorkspaceSurfaceDiff
}

// previousSurfaceFor remembers where a Commit cancel should land.
func (w *workspaceSurfaceState) previousSurfaceFor(from WorkspaceSurfaceKind) WorkspaceSurfaceKind {
	return WorkspaceSurfaceDiff
}

// openCommit implements "Commit action → Commit". The preflight capture is
// flagged; the caller fills it right after (must not render a commit that
// silently trusts a stale snapshot).
func (w *workspaceSurfaceState) openCommit() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.kind == WorkspaceSurfaceCommit {
		return
	}
	w.previous = w.kind
	w.kind = WorkspaceSurfaceCommit
	w.commit = CommitSurfaceState{Selected: map[string]bool{}}
}

// cancelCommit implements "Commit cancel → previous Diff".
func (w *workspaceSurfaceState) cancelCommit() WorkspaceSurfaceKind {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.kind != WorkspaceSurfaceCommit {
		return w.kind
	}
	w.kind = w.previous
	if w.kind == "" {
		w.kind = WorkspaceSurfaceDiff
	}
	w.commit = CommitSurfaceState{}
	return w.kind
}

// commitSucceeded implements "Commit success with changes → Diff refreshed"
// and "Commit success no changes → Diff empty state".
func (w *workspaceSurfaceState) commitSucceeded(remainingChanges bool) WorkspaceSurfaceKind {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.kind != WorkspaceSurfaceCommit {
		return w.kind
	}
	w.kind = WorkspaceSurfaceDiff
	w.commit.ErrText = ""
	return w.kind
}

// branchSwitched keeps the Diff surface and refreshes (plan §6: "switch
// branch → remain Diff + refresh").
func (w *workspaceSurfaceState) branchSwitched() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.kind == WorkspaceSurfaceCommit {
		w.kind = WorkspaceSurfaceDiff
	}
}

// isValid reports whether a stored surface state can be restored for ctx.
func (w *workspaceSurfaceState) isValid(ctx WorkspaceContextKey) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.context == ctx && w.kind != ""
}

// showSurface switches between the Diff review and the Commit editor. There
// is no Terminal to go back to: asking for anything but Commit lands on Diff.
func (s *Shell) showSurface(kind WorkspaceSurfaceKind) {
	if s.surface.current() == WorkspaceSurfaceCommit && kind != WorkspaceSurfaceCommit && s.holdCommitNavigation() {
		return
	}
	if s.surface.current() == kind {
		return
	}
	switch kind {
	case WorkspaceSurfaceCommit:
		s.openCommitSurface()
	default:
		s.surface.openDiff(s.workspaceContext(), s.surface.diff.SelectedPath)
		s.ensureGitSnapshot(false)
	}
}

// setDiffLayout updates only when the requested layout differs, making both
// icon buttons and compact overflow routes obey one source of truth.
func (s *Shell) setDiffLayout(split bool) {
	if s.git == nil || s.git.splitLayout == split {
		return
	}
	s.git.splitLayout = split
	s.applySettings(func(cfg *settings.Settings) error {
		cfg.Workbench.SplitDiff = split
		return nil
	})
	s.rebuildDiffRows()
}

func (s *Shell) toggleDiffLayout() {
	if s.git != nil {
		s.setDiffLayout(!s.git.splitLayout)
	}
}

// openCommitSurface retains the stale-state preflight, but moves the commit
// editor into a native modal over the underlying Diff Review.
func (s *Shell) openCommitSurface() {
	s.gitMore.pushAfterCommit = false
	if s.surface.current() == WorkspaceSurfaceCommit {
		s.commitDialogOpen = true
		return
	}
	s.surface.openCommit()
	s.commitDialogOpen = true
	s.commitDiscardPrompt = false
	snap := s.gitSnapshot()
	if snap == nil {
		s.surface.commit.ErrText = "Changes are still loading."
		return
	}
	// Default selection: all visible changed files.
	for _, cf := range snap.Files {
		s.surface.commit.Selected[cf.Path] = true
	}
	s.captureCommitPreflight()
}

// requestCancelCommit protects message edits and in-flight Git mutations.
func (s *Shell) requestCancelCommit() {
	if s.surface.commit.InFlight || s.git != nil && s.git.committing {
		s.pendingToast = "Commit in progress; wait for it to finish."
		return
	}
	if s.surface.commit.Subject != "" || s.surface.commit.Body != "" {
		s.commitDiscardPrompt = true
		return
	}
	s.cancelCommitSurface()
}

// holdCommitNavigation intercepts intent to leave the sole Commit editor.
// The caller may navigate only after the modal is cleanly cancelled. An
// edited draft asks for deliberate Discard; no view or surface change occurs.
func (s *Shell) holdCommitNavigation() bool {
	if s.surface.current() != WorkspaceSurfaceCommit {
		return false
	}
	s.commitDialogOpen = true
	s.requestCancelCommit()
	return s.surface.current() == WorkspaceSurfaceCommit
}

// cancelCommitSurface leaves Commit back to the Diff review.
func (s *Shell) cancelCommitSurface() {
	s.gitMore.pushAfterCommit = false
	s.commitDialogOpen = false
	s.commitDiscardPrompt = false
	s.surface.cancelCommit()
	s.ensureGitSnapshot(false)
}
