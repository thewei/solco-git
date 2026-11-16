package nativeui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

/**
 * [INPUT]: keymap.go 的键位表与 keymapPass、keymap_help.go 的速查浮层、带裸远程的真实临时仓库、headless ui.Tester 的按键
 * [OUTPUT]: 2026-11-16 键盘 P1 的合同：键位表自洽（同上下文内键不重复、字母键不带 ⌘ ⌃ ⌥）；列表有焦点时 f / p / ⇧P 真的 fetch / pull / push、`?` 打开当前上下文的速查、`/` 打开该列表的搜索；输入框取走文本时这些键让路；J / K 在列表里走行
 * [POS]: 单字母键是 LazyGit 的核心，也是最容易误触的东西：这里先守住「只在列表有焦点时」与「不绕过确认」两条
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

func TestTheKeymapAgreesWithItself(t *testing.T) {
	type slot struct {
		ctx  keyContext
		mods ui.Modifiers
		key  ui.Key
	}
	seen := map[slot]string{}
	for _, a := range keymap {
		if len(a.caps) == 0 || a.label == "" {
			t.Errorf("a keymap row has no keycaps or no label: %+v", a)
		}
		if a.run == nil {
			continue
		}
		// ⌥ with an arrow types nothing, so the arrows may carry it (move a commit);
		// ⌥ with a letter types a character, and ⌘ ⌃ belong to the menu bar.
		arrow := a.key == ui.KeyUp || a.key == ui.KeyDown
		if a.mods&^ui.Shift != 0 && !(arrow && a.mods == ui.Alt) {
			t.Errorf("%q binds a modifier besides ⇧: letters with ⌘ ⌃ ⌥ belong to the menu bar (⌥ also types a character on macOS)", a.label)
		}
		s := slot{a.ctx, a.mods, a.key}
		if other, dup := seen[s]; dup {
			t.Errorf("%q and %q share a key in %s", a.label, other, a.ctx.title())
		}
		seen[s] = a.label
		// a key bound anywhere cannot be bound again inside one context
		if a.ctx != keysAny {
			if other, dup := seen[slot{keysAny, a.mods, a.key}]; dup {
				t.Errorf("%q hides %q, which answers anywhere", a.label, other)
			}
		}
	}
	// the "anywhere" rows are checked against the contexts once all are in
	for s, label := range seen {
		if s.ctx == keysAny {
			continue
		}
		if other, dup := seen[slot{keysAny, s.mods, s.key}]; dup {
			t.Errorf("%q hides %q, which answers anywhere", label, other)
		}
	}
}

func TestEveryContextHasASheet(t *testing.T) {
	for _, ctx := range []keyContext{keysChanges, keysCommits, keysRefs, keysFiles, keysCommitFiles, keysDiff} {
		own, anywhere := keysIn(ctx)
		if len(own) == 0 {
			t.Errorf("%s has no rows of its own in the keyboard sheet", ctx.title())
		}
		if len(anywhere) == 0 {
			t.Errorf("%s shows none of the keys that answer anywhere", ctx.title())
		}
	}
}

// simpleRemote is a work repository on main with a bare origin and a second
// clone that can push to it.
func simpleRemote(t *testing.T) (root, bare, other string) {
	t.Helper()
	base := t.TempDir()
	bare = filepath.Join(base, "origin.git")
	if err := os.MkdirAll(bare, 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, bare, "init", "-q", "--bare", "-b", "main")
	root = filepath.Join(base, "work")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "init", "-q", "-b", "main")
	gitRun(t, root, "remote", "add", "origin", bare)
	writeFile(t, filepath.Join(root, "a.txt"), "one\n")
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "one")
	gitRun(t, root, "push", "-q", "-u", "origin", "main")
	other = filepath.Join(base, "other")
	gitRun(t, base, "clone", "-q", bare, other)
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	return root, bare, other
}

// commitListShell opens root on the Commits view with the commit list holding
// the keyboard (a click on the newest commit's row).
func commitListShell(t *testing.T, root string) (*ui.Tester, *uiPump, *Shell) {
	t.Helper()
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	sh.showView(viewCommits)
	pump.until(t, "commits", func() bool { return len(sh.git.commits) > 0 })
	tt := newTester(w, 1280, 800, true)
	settle(tt, pump, 3)
	if err := tt.Click(sh.git.commits[0].Subject); err != nil {
		t.Fatalf("click a commit: %v", err)
	}
	settle(tt, pump, 3)
	return tt, pump, sh
}

func TestListKeysFetchPullAndPush(t *testing.T) {
	root, bare, other := simpleRemote(t)
	tt, pump, sh := commitListShell(t, root)

	// Someone else pushes; F fetches it.
	writeFile(t, filepath.Join(other, "b.txt"), "two\n")
	gitRun(t, other, "add", ".")
	gitRun(t, other, "-c", "user.name=Other", "-c", "user.email=o@example.com", "commit", "-q", "-m", "two")
	gitRun(t, other, "push", "-q", "origin", "main")
	want := gitRun(t, bare, "rev-parse", "main")
	tt.Key(0, ui.KeyF)
	pump.until(t, "F to fetch", func() bool { settle(tt, pump, 1); return originMain(t, sh) == want })
	if head := gitRun(t, root, "rev-parse", "HEAD"); head == want {
		t.Fatal("fetch must not move HEAD")
	}

	// P pulls it (fast-forward). A key pressed while an operation still runs is
	// refused with a toast, so each waits for the one before to finish.
	pump.until(t, "the fetch to finish", func() bool { settle(tt, pump, 1); return !sh.git.opBusy })
	tt.Key(0, ui.KeyP)
	pump.until(t, "P to pull", func() bool { settle(tt, pump, 1); return gitRun(t, root, "rev-parse", "HEAD") == want })

	// ⇧P pushes a new local commit.
	writeFile(t, filepath.Join(root, "c.txt"), "three\n")
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "three")
	local := gitRun(t, root, "rev-parse", "HEAD")
	pump.until(t, "the pull to finish", func() bool { settle(tt, pump, 1); return !sh.git.opBusy })
	tt.Key(ui.Shift, ui.KeyP)
	pump.until(t, "⇧P to push", func() bool { settle(tt, pump, 1); return gitRun(t, bare, "rev-parse", "main") == local })
}

func TestQuestionMarkOpensTheSheetOfTheFocusedList(t *testing.T) {
	root := gitFixture(t, "keys-help")
	tt, pump, sh := commitListShell(t, root)
	tt.Key(ui.Shift, ui.KeySlash)
	settle(tt, pump, 3)
	if !sh.gitMore.keys.open || sh.gitMore.keys.ctx != keysCommits {
		t.Fatalf("? opened %+v, want the Commits sheet", sh.gitMore.keys)
	}
	for _, want := range []string{"Keyboard Shortcuts", "Search commits", "Fetch", "Push"} {
		if !tt.HasText(want) {
			t.Errorf("the sheet lacks %q; texts: %q", want, tt.Texts())
		}
	}
}

func TestSlashOpensTheSearchOfTheFocusedList(t *testing.T) {
	root := gitFixture(t, "keys-slash")
	tt, pump, sh := commitListShell(t, root)
	if sh.gitCommitSearchOpen {
		t.Fatal("the commit search starts closed")
	}
	tt.Key(0, ui.KeySlash)
	settle(tt, pump, 3)
	if !sh.gitCommitSearchOpen {
		t.Fatal("/ in the commit list must open the commit search")
	}
}

// Typing in a field is typing: the list keys never reach it.
func TestListKeysStepAsideWhileAFieldTakesText(t *testing.T) {
	root := gitFixture(t, "keys-typing")
	w, pump := newTestWindow(t)
	tt := newTester(w, 1280, 800, true)
	sh := openAndLoad(t, w, pump, root)
	pump.until(t, "the changes", func() bool { settle(tt, pump, 1); return len(sh.git.gdFiles) > 0 })
	if err := tt.Click("Commit subject draft"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 1)
	for _, k := range []struct {
		mods ui.Modifiers
		key  ui.Key
		text string
	}{{0, ui.KeyF, "f"}, {0, ui.KeyP, "p"}, {0, ui.KeyZ, "z"}, {ui.Shift, ui.KeyP, "P"}, {ui.Shift, ui.KeySlash, "?"}} {
		tt.TypeKey(k.mods, k.key, k.text)
	}
	settle(tt, pump, 2)
	if sh.gitQuickSubject != "fpzP?" {
		t.Errorf("subject = %q, want every key typed as text", sh.gitQuickSubject)
	}
	if sh.gitMore.keys.open || sh.git.opBusy {
		t.Errorf("a list key fired while typing: sheet open=%v opBusy=%v", sh.gitMore.keys.open, sh.git.opBusy)
	}
	if strings.TrimSpace(sh.pendingToast) != "" {
		t.Errorf("a list key spoke while typing: %q", sh.pendingToast)
	}
}

func TestJAndKWalkTheRowsOfAList(t *testing.T) {
	root := gitFixture(t, "keys-jk")
	tt, pump, sh := commitListShell(t, root)
	first := reviewedHash(sh)
	if first == "" {
		t.Fatal("the click reviews a commit")
	}
	tt.Key(0, ui.KeyJ)
	settle(tt, pump, 2)
	pump.until(t, "J to follow", func() bool { return reviewedHash(sh) != first && reviewedHash(sh) != "" })
	tt.Key(0, ui.KeyK)
	settle(tt, pump, 2)
	pump.until(t, "K to come back", func() bool { return reviewedHash(sh) == first })
}

func TestJAndKWalkTheChangedFiles(t *testing.T) {
	root := gitFixture(t, "keys-jk-changes")
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	tt := newTester(w, 1280, 800, true)
	pump.until(t, "the changes", func() bool { settle(tt, pump, 1); return len(sh.git.gdFiles) > 0 })
	if err := tt.Click("README.md"); err != nil {
		t.Fatalf("click a changed file: %v", err)
	}
	settle(tt, pump, 3)
	first := sh.surface.diff.SelectedPath
	tt.Key(0, ui.KeyK) // README.md is the last unstaged row
	settle(tt, pump, 3)
	if got := sh.surface.diff.SelectedPath; got == first || got == "" {
		t.Fatalf("K left the selection on %q; it must move up a file", got)
	}
	tt.Key(0, ui.KeyJ)
	settle(tt, pump, 3)
	if got := sh.surface.diff.SelectedPath; got != first {
		t.Fatalf("J did not come back: selection is %q, want %q", got, first)
	}
}

func TestJAndKWalkTheReferencesTree(t *testing.T) {
	w, pump, sh := focusShell(t)
	tt := newTester(w, 1280, 800, false)
	settle(tt, pump, 3)
	for i := 0; i < 5 && sh.nav.gitRefs.cursor == ""; i++ {
		tt.Key(0, ui.KeyTab)
		settle(tt, pump, 3)
	}
	start := sh.nav.gitRefs.cursor
	if start == "" {
		t.Skip("Tab did not reach the references tree")
	}
	tt.Key(0, ui.KeyJ)
	settle(tt, pump, 3)
	moved := sh.nav.gitRefs.cursor
	if moved == start || moved == "" {
		t.Fatalf("J left the cursor on %q; it must step down a row", moved)
	}
	tt.Key(0, ui.KeyK)
	settle(tt, pump, 3)
	if got := sh.nav.gitRefs.cursor; got != start {
		t.Fatalf("K did not come back: cursor %q, want %q", got, start)
	}
}

// F means Fetch in a list and nothing in the diff: the letter belongs to the
// surface that holds the keyboard.
func TestAListKeyDoesNothingWhileTheDiffHoldsTheKeyboard(t *testing.T) {
	root, bare, other := simpleRemote(t)
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	tt := newTester(w, 1280, 800, true)
	writeFile(t, filepath.Join(root, "a.txt"), "one\ntwo\nthree\n")
	sh.refreshGitChanges()
	pump.until(t, "the changed file", func() bool { settle(tt, pump, 1); return len(sh.git.gdFiles) > 0 })
	writeFile(t, filepath.Join(other, "b.txt"), "x\n")
	gitRun(t, other, "add", ".")
	gitRun(t, other, "-c", "user.name=Other", "-c", "user.email=o@example.com", "commit", "-q", "-m", "elsewhere")
	gitRun(t, other, "push", "-q", "origin", "main")
	before := originMain(t, sh)
	if before == gitRun(t, bare, "rev-parse", "main") {
		t.Fatal("the remote must be ahead for this check to mean something")
	}
	if err := tt.Click("three"); err != nil {
		t.Skipf("no diff line to click: %v", err)
	}
	settle(tt, pump, 3)
	tt.Key(0, ui.KeyF)
	idle(pump)
	settle(tt, pump, 2)
	if got := originMain(t, sh); got != before {
		t.Error("F fetched while the keyboard was in the diff")
	}
}

// J / K step hunks only while the diff holds the keyboard.
func TestJAndKStepHunksWhileTheDiffHoldsTheKeyboard(t *testing.T) {
	root := gitFixture(t, "keys-hunks")
	var lines []string
	for i := 1; i <= 60; i++ {
		lines = append(lines, "row-"+itoa(i))
	}
	writeFile(t, filepath.Join(root, "rows.txt"), strings.Join(lines, "\n")+"\n")
	gitRun(t, root, "add", "rows.txt")
	gitRun(t, root, "commit", "-q", "-m", "rows")
	lines[2], lines[54] = "changed-3", "changed-55"
	writeFile(t, filepath.Join(root, "rows.txt"), strings.Join(lines, "\n")+"\n")

	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	tt := newTester(w, 1280, 800, true)
	pump.until(t, "the diff", func() bool { settle(tt, pump, 1); return len(sh.git.gdRows) > 0 })
	if err := tt.Click("changed-3"); err != nil {
		t.Skipf("no diff line to click: %v", err)
	}
	settle(tt, pump, 3)
	tt.Key(0, ui.KeyJ)
	settle(tt, pump, 3)
	f1, h1 := sh.git.gdSelFile, sh.git.gdSelHunk
	if f1 < 0 {
		t.Fatal("J in the diff must select a hunk")
	}
	tt.Key(0, ui.KeyJ)
	settle(tt, pump, 3)
	if sh.git.gdSelFile == f1 && sh.git.gdSelHunk == h1 {
		t.Fatal("a second J must move on to the next hunk")
	}
}

// Enter takes the keyboard into the diff, Esc gives it back to the list.
func TestEnterEntersTheDiffAndEscComesBack(t *testing.T) {
	root := gitFixture(t, "keys-enter")
	tt, pump, sh := commitListShell(t, root)
	pump.until(t, "the commit's diff", func() bool { settle(tt, pump, 1); return len(sh.git.gdRows) > 0 })

	tt.Key(0, ui.KeyEnter)
	settle(tt, pump, 3)
	// J now steps hunks, not commits: the review stays on the same commit.
	before := reviewedHash(sh)
	tt.Key(0, ui.KeyJ)
	settle(tt, pump, 3)
	idle(pump)
	if got := reviewedHash(sh); got != before {
		t.Fatalf("after Enter, J moved the commit list (%q -> %q); the keyboard should be in the diff", before, got)
	}
	if sh.git.gdSelFile < 0 {
		t.Fatal("after Enter, J must step a hunk in the diff")
	}

	tt.Key(0, ui.KeyEscape)
	settle(tt, pump, 3)
	tt.Key(0, ui.KeyJ)
	settle(tt, pump, 3)
	pump.until(t, "J to walk the commits again after Esc", func() bool { return reviewedHash(sh) != before })
}
