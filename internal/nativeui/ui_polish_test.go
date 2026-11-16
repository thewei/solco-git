package nativeui

/**
 * [INPUT]: icons.go 的框形图标、git_sidebar_tree.go 的分组图标、gorex_style.go 的 middlePath、sidebar.go 的作者列分档、window_dialogs.go 的设置滚动区、gd_surface.go 的提交加载态、真实临时仓库
 * [OUTPUT]: 2026-11-16 界面打磨批次的合同：三个框形图标各不相同、引用分组头带自己的图标且标题与行标题对齐、长路径从中间收短而保留末尾、作者列在窄中心只剩头像、设置滚动区随窗口增高、点开提交时只有一处「正在读」
 * [POS]: UI / UX / 图标评审的落地；每条规则做过变异检查
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"strings"
	"testing"
)

// The side panel, Split and Unified glyphs stand together in the title band
// and the diff toolbar; the same box with a line in three places read alike.
func TestFrameGlyphsAreThreeDifferentShapes(t *testing.T) {
	shapes := map[string]string{"panel": iconPanelShapes, "split": iconSplitShapes, "unified": iconUnifiedShapes}
	seen := map[string]string{}
	for name, s := range shapes {
		if other, dup := seen[s]; dup {
			t.Errorf("%s and %s are the same drawing", name, other)
		}
		seen[s] = name
	}
	if iconUnifiedShapes == `<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 12h18"/>` {
		t.Error("Unified must not be the box cut in two rows: that reads as a top/bottom split")
	}
}

// Every reference group wears the icon of its rows, and its title lines up
// with their titles (the glyph sits where a row's glyph does).
func TestReferenceGroupsWearTheirRowsIcon(t *testing.T) {
	for _, name := range []string{"Worktrees", "Branches", "Remotes", "Tags", "Stashes", "Submodules"} {
		if gitSectionGlyph(name) == nil {
			t.Errorf("group %q has no icon", name)
		}
	}
	if gitSectionGlyph("Something else") != nil {
		t.Error("an unknown group should draw no icon")
	}

	root := gitFixture(t, "groups")
	w, pump := newTestWindow(t)
	openAndLoad(t, w, pump, root)
	tt := newTester(w, 1280, 800, true)
	pump.until(t, "the references", func() bool { return len(w.workspaces[0].sh.git.branches) > 0 })
	settle(tt, pump, 4)
	header, ok := tt.Find("Branches")
	if !ok {
		t.Fatalf("no Branches header: %q", tt.Texts())
	}
	row, ok := tt.Find("feature")
	if !ok {
		t.Fatalf("no branch row: %q", tt.Texts())
	}
	if header.X != row.X {
		t.Errorf("the group title (x %.0f) and its rows' titles (x %.0f) should line up", header.X, row.X)
	}
}

func TestMiddlePathKeepsTheEnd(t *testing.T) {
	for _, tc := range []struct {
		in   string
		max  int
		want string
	}{
		{"~/Workspaces/solco-git", 46, "~/Workspaces/solco-git"},
		{"~/Workspaces/wh-studio/clients/solco-git", 30, "~/…/clients/solco-git"},
		{"/Users/me/Workspaces/wh-studio/clients/solco-git", 30, "/Users/…/clients/solco-git"},
		{"/Users/me/Workspaces/wh-studio/clients/solco-git", 46, "/Users/…/wh-studio/clients/solco-git"},
		{"~/a/b", 3, "~/a/b"}, // nothing in the middle to give
		{"~/Workspaces/a-very-long-repository-folder-name", 10, "~/Workspaces/a-very-long-repository-folder-name"},
		{"/private/var/folders/rd/abc/T/TestWindow/001/app", 40, "/private/…/rd/abc/T/TestWindow/001/app"},
		{"/private/var/folders/rd/abc/T/TestWindow/001/app", 30, "/private/…/TestWindow/001/app"},
		{"", 10, ""},
	} {
		if got := middlePath(tc.in, tc.max); got != tc.want {
			t.Errorf("middlePath(%q, %d) = %q, want %q", tc.in, tc.max, got, tc.want)
		}
	}
	// The last folder always survives, however long.
	if got := middlePath("~/aaa/bbb/ccc/ddd-the-repository-itself", 12); !strings.HasSuffix(got, "ddd-the-repository-itself") {
		t.Errorf("the repository's own folder was cut: %q", got)
	}
}

// The author column keeps its name only where it fits: below that it is the
// avatar alone (the name stays in its tip) rather than "Ada Lovela…".
func TestAuthorNameNeedsRoom(t *testing.T) {
	root := gitFixture(t, "authors")
	for _, tc := range []struct {
		width int
		named bool
	}{{1280, true}, {1100, false}} {
		w, pump := newTestWindow(t)
		sh := openAndLoad(t, w, pump, root)
		tt := newTester(w, tc.width, 800, true)
		sh.showGitCommits()
		pump.until(t, "the commit list", func() bool { return len(sh.git.commits) > 0 })
		settle(tt, pump, 3)
		if got := tt.HasText("Test"); got != tc.named {
			t.Errorf("at %d wide the author name shows = %v, want %v (center %.0f)", tc.width, got, tc.named, sh.gitCenterWidth(float32(tc.width)))
		}
		if tc.width == 1100 {
			if c := sh.gitCenterWidth(1100); c < gitCommitsShowAuthor || c >= gitCommitsShowAuthorName {
				t.Fatalf("the test width no longer lands between the author tiers: center %.0f", c)
			}
		}
	}
}

// Settings grows with a tall window; at the common height it is what it was.
// The list's own height is the distance from its first group to Done.
func TestSettingsListFollowsTheWindowHeight(t *testing.T) {
	listHeight := func(height int) float32 {
		root := gitFixture(t, "settings")
		w, pump := newTestWindow(t)
		openAndLoad(t, w, pump, root)
		w.settingsOpen = true
		tt := newTester(w, 1280, height, true)
		settle(tt, pump, 4)
		done, ok := tt.Find("Done")
		if !ok {
			t.Fatalf("no Done button at height %d: %q", height, tt.Texts())
		}
		first, ok := tt.Find("Appearance")
		if !ok {
			t.Fatalf("no Appearance group at height %d: %q", height, tt.Texts())
		}
		return done.Y - first.Y
	}
	short, tall := listHeight(700), listHeight(1100)
	if tall < short+150 {
		t.Errorf("a window 400 points taller should give the list room: %.0f → %.0f points from its first group to Done", short, tall)
	}
}

// Clicking a commit shows one "reading" — the strip's — not the diff and the
// file column saying it again.
func TestOpeningACommitLoadsInOnePlace(t *testing.T) {
	root := historyRepo(t, "one-spinner", 3)
	sh, pump, tt := historyShell(t, root)
	sh.reviewCommit(hashOf(t, root, "c2"))
	tt.Frame() // the frame before the read comes back
	if sh.git.commitSnap != nil {
		t.Skip("the read finished before the first frame")
	}
	if tt.HasText("Loading changes…") {
		t.Errorf("the diff repeats the strip's \"Reading commit…\": %q", tt.Texts())
	}
	if !tt.HasText("Reading commit…") {
		t.Errorf("the strip must still say it is reading: %q", tt.Texts())
	}
	settle(tt, pump, 3)
}
