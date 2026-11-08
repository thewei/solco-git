// Package settings is Solco Git's own preference store: a few Git workbench
// cosmetics and the window's restorable state (the workspaces). It keeps
// its own file under the app's user-data directory and imports nothing from
// Solco (2026-11-07 decision, item 4).
//
// [INPUT]: MyGo's per-app user-data directory.
// [OUTPUT]: Settings / Store / FileStore / MemoryStore, plus the cosmetics the Git workbench reads (Auto-fetch, Split diff, hidden refs, folded merges, ref-tree folds, Hide Stashes).
// [POS]: persistence only; Git truth stays in solco-gitkit and the repository itself.
package settings

import (
	"context"
	"sync"
)

// CurrentSchemaVersion is bumped when a field changes meaning.
const CurrentSchemaVersion = 1

// Settings is the whole persisted document.
type Settings struct {
	SchemaVersion int               `json:"schema_version"`
	General       GeneralSettings   `json:"general"`
	Workbench     WorkbenchSettings `json:"workbench"`
	Window        WindowSettings    `json:"window"`
}

// GeneralSettings are the app-level switches.
type GeneralSettings struct {
	// Appearance is "system" (follow macOS, the default), "light" or "dark".
	Appearance string `json:"appearance"`
	// RestoreWindow keeps the last window bounds between launches.
	RestoreWindow bool `json:"restore_window"`
	// Density configures the sidebar spacing: "compact", "default", "comfortable".
	Density string `json:"density,omitempty"`
	// HighContrast ("Increase contrast") swaps the muted separators and text
	// for stronger ones, for the whole app.
	HighContrast bool `json:"high_contrast,omitempty"`
}

// WorkbenchSettings are the cosmetics the Git workbench remembers. Git itself
// decides what each one means; the client only stores the choice.
type WorkbenchSettings struct {
	// AutoFetchMinutes, when 5/15/30/60, fetches the open repository's remote
	// that often in the background. 0 = off (default). Fetch only: never
	// merges or pushes.
	AutoFetchMinutes int `json:"auto_fetch_minutes,omitempty"`
	// SplitDiff persists the unified/split diff layout choice.
	SplitDiff bool `json:"split_diff,omitempty"`
	// GraphHidden maps a repository root to the full ref names the user hid
	// from the Commits graph (the sidebar eye).
	GraphHidden map[string][]string `json:"graph_hidden,omitempty"`
	// GraphFolded maps a repository root to the merge commits (full hashes)
	// whose merged-in branch the Commits graph folds away.
	GraphFolded map[string][]string `json:"graph_folded,omitempty"`
	// TreeFolds maps a repository root to the sidebar ref-tree rows and
	// sections the user folded or opened away from the defaults (true = folded).
	TreeFolds map[string]map[string]bool `json:"tree_folds,omitempty"`
	// HideStashes drops the stash chips from the Commits list.
	HideStashes bool `json:"hide_stashes,omitempty"`
}

// WindowSettings is the window's restorable state: the workspaces the user
// keeps — the repositories the title band's pull-down lists — and which one
// was in front, plus the repositories opened before (the welcome page and
// Quick Launch list them).
type WindowSettings struct {
	// Workspaces are in the order the pull-down lists them: the user's order,
	// new ones last.
	Workspaces []WorkspaceSettings `json:"workspaces,omitempty"`
	// Tabs is where the builds with a tab strip kept that same list; Migrate
	// folds it into Workspaces, and a save writes only Workspaces.
	Tabs   []WorkspaceSettings `json:"tabs,omitempty"`
	Active int                 `json:"active,omitempty"`
	// Recent holds repository roots, newest first, capped at RecentMax.
	Recent []string `json:"recent,omitempty"`
	// SidebarWidth is the sidebar's dragged width; 0 keeps the default.
	SidebarWidth int `json:"sidebar_width,omitempty"`
}

// Migrate folds the keys an older build wrote into the current ones, so a
// document from the tab-strip era keeps its repositories as workspaces. It is
// idempotent.
func (w *WindowSettings) Migrate() {
	if len(w.Workspaces) == 0 && len(w.Tabs) > 0 {
		w.Workspaces = w.Tabs
	}
	w.Tabs = nil
}

// RecentMax bounds Window.Recent.
const RecentMax = 12

// WorkspaceSettings is one workspace. Root is the repository's top-level
// directory; View is "changes", "commits" or "files".
type WorkspaceSettings struct {
	Root string `json:"root"`
	View string `json:"view,omitempty"`
}

// Default is the document of a first launch.
func Default() Settings {
	return Settings{
		SchemaVersion: CurrentSchemaVersion,
		General:       GeneralSettings{Appearance: "system", RestoreWindow: true},
	}
}

// Store loads and saves the document.
type Store interface {
	Load(context.Context) (Settings, error)
	Save(context.Context, Settings) error
}

// MemoryStore is for tests; production persistence uses FileStore.
type MemoryStore struct {
	mu       sync.RWMutex
	settings Settings
}

// NewMemoryStore starts a store holding initial.
func NewMemoryStore(initial Settings) *MemoryStore { return &MemoryStore{settings: initial} }

// Load returns the held document.
func (s *MemoryStore) Load(ctx context.Context) (Settings, error) {
	if err := ctx.Err(); err != nil {
		return Settings{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings, nil
}

// Save replaces the held document.
func (s *MemoryStore) Save(ctx context.Context, value Settings) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	s.settings = value
	s.mu.Unlock()
	return nil
}
