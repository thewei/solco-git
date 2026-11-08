package nativeui

/**
 * [INPUT]: window.go / window_workspaces.go / external_open.go / quick_launch.go、真实临时仓库、文件型设置存储
 * [OUTPUT]: 窗口层合同：同一仓库（含子目录、其中的文件）只有一个工作区并切到它；带 view=changes 的文件链接在 Changes 里选中它（不落到 Files；仓库外或未改动的路径什么也不选；工作区还没读完仓库时到达的链接，首读落地后滚到该文件的卡）；非仓库给出提示不加工作区；移除到最后一个回欢迎页而不是关窗；有草稿或在跑的 Git 操作先确认；上移/下移保持当前工作区；工作区与当前、最近列表落盘并在下次启动恢复（旧版的 tabs 键读成工作区）；solcogit:// 解析的全部边界；窗口未就绪时到达的链接排队；Quick Launch 的匹配与排序；同名工作区的区分
 * [POS]: 2026-11-07「Solco Git」与 2026-11-08「工作区下拉」决定里"一个窗口 + 工作区清单"的回归防线；全部在没有原生窗口的情况下直接驱动 Window 模型
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"

	"github.com/thewei/solco-git/internal/settings"
)

// window opens each repository in the window the production way.
func openAll(t *testing.T, w *Window, roots ...string) {
	t.Helper()
	for _, root := range roots {
		w.OpenRepository(root, "", "")
	}
}

func TestOpeningARepositoryTwiceSelectsItsWorkspace(t *testing.T) {
	a, b := gitFixture(t, "alpha"), gitFixture(t, "beta")
	w, _ := newTestWindow(t)
	openAll(t, w, a, b)
	if len(w.workspaces) != 2 || w.active != 1 {
		t.Fatalf("two repositories: workspaces=%d active=%d, want 2 and 1", len(w.workspaces), w.active)
	}
	// The same repository by its root, by a folder inside it, and by a file
	// inside it all land on the existing workspace.
	w.OpenRepository(a, "", "")
	if len(w.workspaces) != 2 || w.active != 0 {
		t.Fatalf("reopening alpha: workspaces=%d active=%d, want 2 and 0", len(w.workspaces), w.active)
	}
	w.OpenRepository(filepath.Join(b, "src", "lib"), "", "")
	if len(w.workspaces) != 2 || w.active != 1 {
		t.Fatalf("a folder inside beta: workspaces=%d active=%d, want 2 and 1", len(w.workspaces), w.active)
	}
	w.OpenRepository(filepath.Join(a, "README.md"), "", "")
	if len(w.workspaces) != 2 || w.active != 0 {
		t.Fatalf("a file inside alpha: workspaces=%d active=%d, want 2 and 0", len(w.workspaces), w.active)
	}
}

func TestOpeningAFolderThatIsNotARepositoryOnlyExplains(t *testing.T) {
	w, _ := newTestWindow(t)
	plain := t.TempDir()
	w.OpenRepository(plain, "", "")
	if len(w.workspaces) != 0 || w.active != -1 {
		t.Errorf("a plain folder opened a workspace: workspaces=%d active=%d", len(w.workspaces), w.active)
	}
	if want := "is not a Git repository"; !strings.Contains(w.toast, want) || !strings.Contains(w.toast, filepath.Base(plain)) {
		t.Errorf("toast = %q, want it to name the folder and say %q", w.toast, want)
	}
	w.OpenRepository("relative/path", "", "")
	if w.toast != "Choose a folder to open" || len(w.workspaces) != 0 {
		t.Errorf("a relative path: toast=%q workspaces=%d", w.toast, len(w.workspaces))
	}
	w.OpenRepository("", "", "")
	if len(w.workspaces) != 0 {
		t.Error("an empty path opens nothing")
	}
}

func TestOpeningWithAViewAndAFileShowsThem(t *testing.T) {
	root := gitFixture(t, "views")
	w, _ := newTestWindow(t)
	w.OpenRepository(root, viewCommits, "")
	if got := w.activeShell().currentView(); got != viewCommits {
		t.Errorf("view = %q, want commits", got)
	}
	w.OpenRepository(root, viewFiles, filepath.Join("src", "lib", "util.go"))
	sh := w.activeShell()
	if sh.currentView() != viewFiles || sh.rightPanel.selectedFilePath != filepath.Join(root, "src", "lib", "util.go") {
		t.Errorf("view=%q selected=%q, want files and util.go", sh.currentView(), sh.rightPanel.selectedFilePath)
	}
	if !sh.rightPanel.expandedDirs[filepath.Join(root, "src")] || !sh.rightPanel.expandedDirs[filepath.Join(root, "src", "lib")] {
		t.Error("revealing a file opens the folders above it")
	}
	// A file outside the repository is ignored, not opened.
	w.OpenRepository(root, viewFiles, "/etc/hosts")
	if got := w.activeShell().rightPanel.selectedFilePath; got != filepath.Join(root, "src", "lib", "util.go") {
		t.Errorf("a file outside the repository changed the selection to %q", got)
	}
}

// A link that asks for Changes with a file selects that file in the list, as a
// click on its row does. It must not drag the workspace over to Files: the panel's
// "Open in Solco Git" on Changes with a file selected landed on Files in the
// 2026-11-08 run, because the file was always revealed the Files way.
func TestOpeningChangesWithAFileSelectsItInTheList(t *testing.T) {
	root := gitFixture(t, "changes-link")
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	w.OpenRepository(root, viewFiles, "") // a view for the link to leave
	if sh.currentView() != viewFiles {
		t.Fatalf("setup: view = %q, want files", sh.currentView())
	}

	w.OpenRepository(root, viewChanges, "notes.txt")
	pump.drain()
	if got := sh.currentView(); got != viewChanges {
		t.Errorf("a Changes link with a file left the workspace on %q", got)
	}
	if got := sh.surface.diff.SelectedPath; got != "notes.txt" {
		t.Errorf("selected = %q, want notes.txt", got)
	}

	// Neither a path outside the repository nor a file that is not changed
	// selects anything; the workspace still shows Changes.
	for _, file := range []string{"/etc/hosts", "src/lib/util.go"} {
		w.OpenRepository(root, viewChanges, file)
		pump.drain()
		if got := sh.surface.diff.SelectedPath; got != "notes.txt" {
			t.Errorf("a link naming %s changed the selection to %q", file, got)
		}
		if got := sh.currentView(); got != viewChanges {
			t.Errorf("a link naming %s moved the workspace to %q", file, got)
		}
	}
}

// The same link into a workspace that has not read its repository yet (the app was
// not running when "Open in Solco Git" was pressed): the card still scrolls
// to the file once the first read lands, not only the row's highlight.
func TestChangesLinkScrollsToTheFileOnceTheFirstReadLands(t *testing.T) {
	root := gitFixture(t, "cold-link")
	w, pump := newTestWindow(t)
	w.OpenRepository(root, viewChanges, "notes.txt")
	sh := w.activeShell()
	if sh == nil {
		t.Fatal("the link opened no workspace")
	}
	pump.until(t, "the first read and the reveal", func() bool {
		return sh.git.snapshot != nil && sh.git.revealPath == "notes.txt"
	})
	if got := sh.surface.diff.SelectedPath; got != "notes.txt" {
		t.Errorf("selected = %q, want notes.txt", got)
	}
	if sh.currentView() != viewChanges {
		t.Errorf("view = %q, want changes", sh.currentView())
	}
}

func TestRemovingWorkspacesKeepsAFrontOneAndEndsAtTheWelcomePage(t *testing.T) {
	a, b, c := gitFixture(t, "one"), gitFixture(t, "two"), gitFixture(t, "three")
	w, _ := newTestWindow(t)
	openAll(t, w, a, b, c) // active = 2
	w.removeWorkspace(0)   // an earlier one: the front one keeps its identity
	if w.active != 1 || w.workspaces[w.active].sh.dir != c {
		t.Errorf("after removing an earlier workspace the front is %d (%s), want three at 1", w.active, w.workspaces[w.active].sh.dir)
	}
	w.removeWorkspace(1) // the front one: the one that slid into its place
	if w.active != 0 || w.workspaces[0].sh.dir != b {
		t.Errorf("after removing the front workspace: active=%d workspaces=%d, want two in front", w.active, len(w.workspaces))
	}
	w.requestRemoveWorkspace(w.active) // the last one leaves the welcome page, never closes the window
	if len(w.workspaces) != 0 || w.active != -1 || w.activeShell() != nil {
		t.Errorf("last workspace removed: workspaces=%d active=%d, want the welcome page", len(w.workspaces), w.active)
	}
	// Out-of-range removals are harmless.
	w.removeWorkspace(5)
	w.removeWorkspace(-1)
	// Removing only takes the repository out of the list: it stays in Recent.
	if recent := w.settings.Window.Recent; len(recent) != 3 {
		t.Errorf("recent = %v, want all three repositories still remembered", recent)
	}
}

func TestRemovingAWorkspaceWithADraftOrARunningOpAsksFirst(t *testing.T) {
	root := gitFixture(t, "ask")
	w, _ := newTestWindow(t)
	openAll(t, w, root)
	sh := w.activeShell()
	sh.gitQuickSubject = "half-written message"
	w.requestRemoveWorkspace(0)
	if len(w.workspaces) != 1 || w.removeAsk != w.workspaces[0].id {
		t.Fatalf("a draft must ask before removing: workspaces=%d removeAsk=%d", len(w.workspaces), w.removeAsk)
	}
	w.removeAsk = 0
	sh.gitQuickSubject = ""
	sh.git.opBusy = true
	w.requestRemoveWorkspace(0)
	if len(w.workspaces) != 1 || w.removeAsk == 0 {
		t.Fatalf("a running Git operation must ask before removing: workspaces=%d removeAsk=%d", len(w.workspaces), w.removeAsk)
	}
	w.removeAsk = 0
	sh.git.opBusy = false
	w.requestRemoveWorkspace(0)
	if len(w.workspaces) != 0 {
		t.Error("a quiet workspace is removed without asking")
	}
}

func TestMovingAWorkspaceKeepsTheFrontOne(t *testing.T) {
	a, b, c := gitFixture(t, "one"), gitFixture(t, "two"), gitFixture(t, "three")
	w, _ := newTestWindow(t)
	openAll(t, w, a, b, c)
	w.selectWorkspace(1) // two in front
	order := func() string {
		var names []string
		for _, ws := range w.workspaces {
			names = append(names, filepath.Base(ws.sh.dir))
		}
		return strings.Join(names, ",")
	}
	ids := []int{w.workspaces[0].id, w.workspaces[1].id, w.workspaces[2].id}
	w.moveWorkspace(ids[2], -1) // three up: one,three,two
	if got := order(); got != "one,three,two" || w.workspaces[w.active].sh.dir != b || w.active != 2 {
		t.Errorf("after moving three up: %s, front=%s at %d; want one,three,two with two in front at 2", got, filepath.Base(w.workspaces[w.active].sh.dir), w.active)
	}
	w.moveWorkspace(ids[1], 1)  // two down: it is the last already
	w.moveWorkspace(ids[0], -1) // one up: it is the first already
	if got := order(); got != "one,three,two" {
		t.Errorf("moving past an end changed the order: %s", got)
	}
	w.moveWorkspace(ids[1], -1) // two up: one,two,three
	if got := order(); got != "one,two,three" || w.workspaces[w.active].sh.dir != b {
		t.Errorf("after moving two up: %s; want one,two,three with two in front", got)
	}
	w.moveWorkspace(999, 1) // a workspace that is gone: nothing
	if got := order(); got != "one,two,three" {
		t.Errorf("a missing workspace changed the order: %s", got)
	}
	// The order is what the next launch restores.
	w.saveState()
	if got := w.settings.Window.Workspaces; len(got) != 3 || got[0].Root != a || got[1].Root != b || got[2].Root != c {
		t.Errorf("saved order = %+v", got)
	}
}

func TestSteppingWorkspacesWraps(t *testing.T) {
	w, _ := newTestWindow(t)
	openAll(t, w, gitFixture(t, "one"), gitFixture(t, "two"), gitFixture(t, "three"))
	w.stepWorkspace(1)
	if w.active != 0 {
		t.Errorf("next from the last workspace = %d, want 0", w.active)
	}
	w.stepWorkspace(-1)
	if w.active != 2 {
		t.Errorf("previous from the first workspace = %d, want 2", w.active)
	}
	single, _ := newTestWindow(t)
	openAll(t, single, gitFixture(t, "solo"))
	single.stepWorkspace(1)
	if single.active != 0 {
		t.Error("one workspace has nowhere to step")
	}
}

func TestWorkspacesAndRecentsPersistAndComeBack(t *testing.T) {
	a, b := gitFixture(t, "keep-a"), gitFixture(t, "keep-b")
	gone := gitFixture(t, "will-vanish")
	dir := t.TempDir()

	w, _ := newTestWindow(t)
	w.store = settings.NewFileStore(dir)
	w.OpenRepository(a, viewFiles, "")
	w.OpenRepository(gone, "", "")
	w.OpenRepository(b, viewCommits, "")
	w.selectWorkspace(0)
	w.saveState()

	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	next := NewWindow(settings.NewFileStore(dir))
	next.runner = w.runner
	next.restoreWorkspaces()
	next.finishAttach()
	if len(next.workspaces) != 2 {
		t.Fatalf("restored %d workspaces, want 2 (the vanished repository is skipped)", len(next.workspaces))
	}
	if next.workspaces[0].sh.dir != a || next.workspaces[0].sh.currentView() != viewFiles ||
		next.workspaces[1].sh.dir != b || next.workspaces[1].sh.currentView() != viewCommits {
		t.Errorf("restored workspaces = %s/%s and %s/%s", next.workspaces[0].sh.dir, next.workspaces[0].sh.currentView(), next.workspaces[1].sh.dir, next.workspaces[1].sh.currentView())
	}
	if next.active != 0 {
		t.Errorf("the front workspace was alpha; restored active=%d", next.active)
	}
	recent := next.settings.Window.Recent
	if len(recent) != 3 || recent[0] != b || recent[1] != gone || recent[2] != a {
		t.Errorf("recent = %v, want newest first: keep-b, will-vanish, keep-a", recent)
	}
}

func TestRecentListIsNewestFirstDeduplicatedAndBounded(t *testing.T) {
	w, _ := newTestWindow(t)
	for i := 0; i < settings.RecentMax+4; i++ {
		w.noteRecent("/repos/r" + string(rune('a'+i)))
	}
	w.noteRecent("/repos/rd") // an old one comes back to the front
	recent := w.settings.Window.Recent
	if len(recent) != settings.RecentMax {
		t.Fatalf("recent holds %d, want the cap %d", len(recent), settings.RecentMax)
	}
	if recent[0] != "/repos/rd" {
		t.Errorf("front = %s, want the one just opened", recent[0])
	}
	seen := map[string]bool{}
	for _, r := range recent {
		if seen[r] {
			t.Errorf("%s is listed twice", r)
		}
		seen[r] = true
	}
}

func TestOpenURLParsing(t *testing.T) {
	long := "solcogit://open?repo=/" + strings.Repeat("a", maxOpenURLLen)
	cases := []struct {
		raw  string
		want openRequest
		ok   bool
	}{
		{"solcogit://open?repo=/Users/me/app", openRequest{Repo: "/Users/me/app"}, true},
		{"solcogit://open?repo=/Users/me/app/&view=commits", openRequest{Repo: "/Users/me/app", View: viewCommits}, true},
		{"solcogit://open?repo=/Users/me/app&view=files", openRequest{Repo: "/Users/me/app", View: viewFiles}, true},
		{"solcogit://open?repo=/Users/me/app&view=changes", openRequest{Repo: "/Users/me/app", View: viewChanges}, true},
		{"solcogit://open?repo=/Users/me/app&view=nonsense", openRequest{Repo: "/Users/me/app", View: viewChanges}, true},
		{"solcogit://open?repo=/Users/me/app&file=src/main.go", openRequest{Repo: "/Users/me/app", File: "src/main.go", View: viewFiles}, true},
		{"solcogit://open?repo=/Users/me/app&file=src/main.go&view=changes", openRequest{Repo: "/Users/me/app", File: "src/main.go", View: viewChanges}, true},
		{"solcogit://open?repo=%2FUsers%2Fme%2Fmy%20app", openRequest{Repo: "/Users/me/my app"}, true},
		// The two links Solco itself makes (solco: internal/solcogit TestURLShapeMatchesWhatSolcoGitParses
		// pins the same literals): a change to the link's shape must change both repositories.
		{"solcogit://open?file=src%2Fmain.go&repo=%2FUsers%2Fme%2Fsrc%2Fapp&view=files", openRequest{Repo: "/Users/me/src/app", File: "src/main.go", View: viewFiles}, true},
		{"solcogit://open?repo=%2FUsers%2Fme%2Fsrc%2Fapp&view=changes", openRequest{Repo: "/Users/me/src/app", View: viewChanges}, true},
		{"solcogit://open", openRequest{}, false},
		{"solcogit://open?repo=", openRequest{}, false},
		{"solcogit://open?repo=relative/path", openRequest{}, false},
		{"solcogit://open?repo=%2FUsers%2Fme%00evil", openRequest{}, false},
		{"solcogit://open?repo=/Users/me/app&file=a%00b", openRequest{}, false},
		{"solcogit://other?repo=/Users/me/app", openRequest{}, false},
		{"solco://open?repo=/Users/me/app", openRequest{}, false},
		{"https://open?repo=/Users/me/app", openRequest{}, false},
		{"://", openRequest{}, false},
		{long, openRequest{}, false},
	}
	for _, c := range cases {
		got, ok := parseOpenURL(c.raw)
		if ok != c.ok || got != c.want {
			t.Errorf("parseOpenURL(%.70q) = %+v, %v; want %+v, %v", c.raw, got, ok, c.want, c.ok)
		}
	}
}

func TestLinksThatArriveBeforeTheWindowAreQueued(t *testing.T) {
	a, b := gitFixture(t, "queued-a"), gitFixture(t, "queued-b")
	w, _ := newTestWindow(t)
	w.OpenURL("solcogit://open?repo=" + a + "&view=commits")
	w.OpenPath(b)
	w.OpenURL("garbage")
	if len(w.workspaces) != 0 {
		t.Fatal("nothing opens before the window is ready")
	}
	if len(w.pending) != 2 {
		t.Fatalf("pending = %d, want 2 (the garbage link is dropped)", len(w.pending))
	}
	w.finishAttach()
	if len(w.workspaces) != 2 || w.workspaces[0].sh.dir != a || w.workspaces[0].sh.currentView() != viewCommits || w.workspaces[1].sh.dir != b {
		t.Errorf("after attach: %d tabs", len(w.workspaces))
	}
	if w.active != 1 {
		t.Errorf("the last link ends up in front, active=%d", w.active)
	}
	if len(w.pending) != 0 {
		t.Error("the queue is drained")
	}
	// Once attached, a link opens at once.
	c := gitFixture(t, "live-c")
	w.OpenURL("solcogit://open?repo=" + c)
	if len(w.workspaces) != 3 || w.workspaces[2].sh.dir != c {
		t.Errorf("a link after attach opens immediately; workspaces=%d", len(w.workspaces))
	}
}

func TestWorkspaceLabelsTellSameNamedRepositoriesApart(t *testing.T) {
	w, _ := newTestWindow(t)
	first, second, other := gitFixture(t, "app"), gitFixture(t, "app"), gitFixture(t, "tool")
	openAll(t, w, first, second, other)
	labels := w.workspaceLabels()
	if labels[2] != "tool" {
		t.Errorf("a unique name stays bare: %q", labels[2])
	}
	for i, root := range []string{first, second} {
		want := "app · " + filepath.Base(filepath.Dir(root))
		if labels[i] != want {
			t.Errorf("label %d = %q, want %q", i, labels[i], want)
		}
	}
	if labels[0] == labels[1] {
		t.Error("two repositories named app must not share a label")
	}
}

func TestQuickMatchScoresNameBeforePath(t *testing.T) {
	cases := []struct {
		query, name, path string
		score             int
		ok                bool
	}{
		{"", "solco", "/work/solco", 0, true},
		{"sol", "solco", "/work/solco", 0, true},      // the name starts with it
		{"SOL", "Solco", "/work/solco", 0, true},      // case does not matter
		{"lco", "solco", "/work/solco", 1, true},      // the name contains it
		{"work", "solco", "/work/solco", 2, true},     // only the path does
		{"work sol", "solco", "/work/solco", 2, true}, // one token is path-only
		{"sol co", "solco", "/work/solco", 0, true},   // every token is in the name
		{"nope", "solco", "/work/solco", 0, false},
		{"sol nope", "solco", "/work/solco", 0, false},
	}
	for _, c := range cases {
		score, ok := quickMatch(c.query, c.name, c.path)
		if score != c.score || ok != c.ok {
			t.Errorf("quickMatch(%q, %q, %q) = %d, %v; want %d, %v", c.query, c.name, c.path, score, ok, c.score, c.ok)
		}
	}
}

func TestQuickItemsOrderAndTypedPath(t *testing.T) {
	open := []quickItem{{Name: "alpha", Path: "/w/alpha", Open: true, WorkspaceID: 1}, {Name: "my-beta", Path: "/w/my-beta", Open: true, WorkspaceID: 2}}
	recent := []quickItem{{Name: "beta-old", Path: "/w/beta-old"}, {Name: "gamma", Path: "/w/gamma"}}
	names := func(items []quickItem) string {
		var out []string
		for _, it := range items {
			out = append(out, it.Name)
		}
		return strings.Join(out, ",")
	}
	if got := names(quickItems(open, recent, "")); got != "alpha,my-beta,beta-old,gamma" {
		t.Errorf("empty query = %s: workspaces first, then recents, as they were", got)
	}
	// "beta": my-beta contains it, beta-old starts with it — each section is
	// ordered by match, the sections stay in order.
	if got := names(quickItems(open, recent, "beta")); got != "my-beta,beta-old" {
		t.Errorf("query beta = %s", got)
	}
	items := quickItems(open, recent, "/tmp/some/where")
	if len(items) == 0 || !items[0].Typed || items[0].Path != "/tmp/some/where" || items[0].Name != "where" {
		t.Errorf("a typed absolute path leads the list as itself: %+v", items)
	}
	if items := quickItems(open, recent, "~/code"); len(items) == 0 || !items[0].Typed || !filepath.IsAbs(items[0].Path) {
		t.Errorf("a typed ~ path is expanded: %+v", items)
	}
}

func TestQuickRowsListWorkspacesThenRecentsNotInTheList(t *testing.T) {
	a, b := gitFixture(t, "rows-a"), gitFixture(t, "rows-b")
	w, _ := newTestWindow(t)
	openAll(t, w, a)
	w.noteRecent(b)
	open, recent := w.quickRows()
	if len(open) != 1 || open[0].Path != a || !open[0].Open {
		t.Errorf("open rows = %+v", open)
	}
	if len(recent) != 1 || recent[0].Path != b || recent[0].Open {
		t.Errorf("recent rows = %+v (a workspace is not listed twice)", recent)
	}
	w.launch(recent[0])
	if len(w.workspaces) != 2 || w.active != 1 {
		t.Errorf("launching a recent opens it: workspaces=%d active=%d", len(w.workspaces), w.active)
	}
	w.launch(open[0])
	if w.active != 0 || len(w.workspaces) != 2 {
		t.Errorf("launching an open row switches to it: active=%d", w.active)
	}
}

func TestSplitMatchCutsAroundTheFirstToken(t *testing.T) {
	cases := []struct{ name, query, pre, mid, post string }{
		{"solco-git", "", "solco-git", "", ""},
		{"solco-git", "git", "solco-", "git", ""},
		{"Solco-Git", "git", "Solco-", "Git", ""},
		{"solco-git", "sol", "", "sol", "co-git"},
		{"solco-git", "zzz", "solco-git", "", ""},
		{"solco-git", "git sol", "solco-", "git", ""}, // the first token only
		{"İstanbul", "ist", "İstanbul", "", ""},       // a case fold that changes length is not sliced
	}
	for _, c := range cases {
		pre, mid, post := splitMatch(c.name, c.query)
		if pre != c.pre || mid != c.mid || post != c.post {
			t.Errorf("splitMatch(%q, %q) = %q|%q|%q, want %q|%q|%q", c.name, c.query, pre, mid, post, c.pre, c.mid, c.post)
		}
	}
}

func TestFetchedAgoReadsNaturally(t *testing.T) {
	cases := map[time.Duration]string{
		0:                        "just now",
		59 * time.Second:         "just now",
		time.Minute:              "1 min ago",
		5 * time.Minute:          "5 min ago",
		time.Hour:                "1 hr ago",
		23 * time.Hour:           "23 hr ago",
		24 * time.Hour:           "1 day ago",
		3 * 24 * time.Hour:       "3 days ago",
		45 * 24 * time.Hour:      "1 month ago",
		200 * 24 * time.Hour:     "6 months ago",
		366 * 24 * time.Hour:     "1 year ago",
		3 * 365 * 24 * time.Hour: "3 years ago",
	}
	for d, want := range cases {
		if got := fetchedAgo(d); got != want {
			t.Errorf("fetchedAgo(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestAcceleratorKeycaps(t *testing.T) {
	cases := map[string]string{
		"CmdOrCtrl+O":       "⌘ O",
		"CmdOrCtrl+Shift+O": "⇧ ⌘ O",
		"CmdOrCtrl+Alt+C":   "⌥ ⌘ C",
		"Ctrl+Shift+Tab":    "⌃ ⇧ ⇥",
		"CmdOrCtrl+[":       "⌘ [",
		"CmdOrCtrl+1":       "⌘ 1",
		"Shift+CmdOrCtrl+W": "⇧ ⌘ W",
	}
	for accelerator, want := range cases {
		if got := strings.Join(acceleratorKeycaps(accelerator), " "); got != want {
			t.Errorf("acceleratorKeycaps(%q) = %q, want %q", accelerator, got, want)
		}
	}
}

func TestNormalizeSettingsClampsToWhatTheUIOffers(t *testing.T) {
	cfg := settings.Default()
	cfg.General.Appearance = "sepia"
	cfg.General.Density = "huge"
	cfg.Workbench.AutoFetchMinutes = 7
	for i := 0; i < settings.RecentMax+5; i++ {
		cfg.Window.Recent = append(cfg.Window.Recent, "/r/"+string(rune('a'+i)))
	}
	got := normalizeSettings(cfg)
	if got.General.Appearance != "system" || got.General.Density != "" || got.Workbench.AutoFetchMinutes != 0 {
		t.Errorf("unknown values were not reset: %+v %+v", got.General, got.Workbench)
	}
	if len(got.Window.Recent) != settings.RecentMax {
		t.Errorf("recent = %d, want %d", len(got.Window.Recent), settings.RecentMax)
	}
	for _, minutes := range []int{0, 5, 15, 30, 60} {
		cfg.Workbench.AutoFetchMinutes = minutes
		if normalizeSettings(cfg).Workbench.AutoFetchMinutes != minutes {
			t.Errorf("auto-fetch %d min is a choice the UI offers and must survive", minutes)
		}
	}
}

// A workspace shows its branch chip from the first frame, before Git has answered.
// The chip must not call a repository on "main" a detached head for that beat
// (seen on a real window's first frame).
func TestBranchChipDoesNotClaimADetachedHeadBeforeGitAnswers(t *testing.T) {
	root := gitFixture(t, "chip")
	w, pump := newTestWindow(t)
	w.OpenRepository(root, "", "")
	i := w.indexOfRoot(root)
	if i < 0 {
		t.Fatal("repository did not open")
	}
	sh := w.workspaces[i].sh
	if sh.git.snapshot != nil {
		t.Fatal("the first read already landed; the test would prove nothing")
	}
	tt := newTester(w, 1280, 800, true)
	tt.Frame()
	if tt.HasText("Detached HEAD") {
		t.Errorf("the chip claims a detached head before Git answered; texts: %q", tt.Texts())
	}
	pump.until(t, "the snapshot", func() bool { return sh.git.snapshot != nil })
	settle(tt, pump, 2)
	if tt.HasText("Detached HEAD") || !tt.HasText("main") {
		t.Errorf("after the read the chip should name the branch; texts: %q", tt.Texts())
	}
}

// Settings → Theme must reach the framework's theme, not only the colours this
// package picks per frame: a Light choice on a dark Mac otherwise left the
// window's sidebar material dark under light text (seen in a real window).
func TestThemePreferenceMapsOntoTheFrameworkThemeSource(t *testing.T) {
	for appearance, want := range map[string]mygo.ThemeSource{
		"light":  mygo.ThemeLight,
		"dark":   mygo.ThemeDark,
		"system": mygo.ThemeSystem,
		"":       mygo.ThemeSystem,
		"sepia":  mygo.ThemeSystem,
	} {
		if got := themeSourceFor(appearance); got != want {
			t.Errorf("themeSourceFor(%q) = %v, want %v", appearance, got, want)
		}
	}
	// A window without a native window leaves the framework alone.
	w, _ := newTestWindow(t)
	w.change(func(cfg *settings.Settings) { cfg.General.Appearance = "dark" })
	if got := mygo.Theme.Source(); got != mygo.ThemeSystem {
		t.Errorf("a headless window changed the framework theme source to %v", got)
	}
}
