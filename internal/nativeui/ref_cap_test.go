package nativeui

/**
 * [INPUT]: git_ref_tree.go 的 gitRenderRefTree（侧栏 Branches / Remotes / Tags 的行上限与「Show N more」）、有几百个分支的真实临时仓库、headless ui.Tester
 * [OUTPUT]: 2026-11-16 的合同：一个分组默认最多画 100 行（折叠的文件夹只算一行），其余收进一行「Show N more」，点开后全部画出、并可「Show fewer」收回；搜索时按全部匹配画、不设上限；不到 100 行的分组什么都不加
 * [POS]: 侧栏不是虚拟列表，每帧把所有行都建一遍：600 个分支让稳态帧从 5 ms 涨到 24 ms；这道上限是它的防线
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"fmt"
	"regexp"
	"testing"
)

// moreRows are the cap's own rows, nothing else that starts with "Show".
var moreRow = regexp.MustCompile(`^(Show -?\d+ more|Show fewer)$`)

func moreRows(texts []string) (found []string) {
	for _, text := range texts {
		if moreRow.MatchString(text) {
			found = append(found, text)
		}
	}
	return
}

// clickInSidebar scrolls the sidebar to the row and clicks it.
func clickInSidebar(t *testing.T, tt interface {
	Scroll(x, y, dx, dy float32)
	Click(string) error
}, label string) {
	t.Helper()
	var err error
	for i := 0; i < 6; i++ {
		if err = tt.Click(label); err == nil {
			return
		}
		tt.Scroll(120, 300, 0, 2000)
	}
	t.Fatalf("click %q: %v", label, err)
}

// manyBranches makes n flat branches b000… on the fixture's HEAD commit.
func manyBranches(t *testing.T, root string, n int) {
	t.Helper()
	head := gitRun(t, root, "rev-parse", "HEAD")
	for i := 0; i < n; i++ {
		gitRun(t, root, "update-ref", fmt.Sprintf("refs/heads/b%03d", i), head)
	}
}

func branchesShell(t *testing.T, n int) (*Window, *uiPump, *Shell) {
	t.Helper()
	root := gitFixture(t, "many")
	manyBranches(t, root, n)
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	pump.until(t, "the references", func() bool { return sh.git.refsRead && len(sh.git.branches) >= n })
	return w, pump, sh
}

func TestALongBranchListStopsAtAHundredRows(t *testing.T) {
	w, pump, sh := branchesShell(t, 250)
	tt := newTester(w, 1280, 800, true)
	settle(tt, pump, 3)
	total := len(sh.git.branches)
	hidden := total - gitRefRowCap
	more := fmt.Sprintf("Show %d more", hidden)
	if !tt.HasText(more) {
		t.Fatalf("no %q row for %d branches; texts: %q", more, total, tt.Texts())
	}
	if !tt.HasText("b000") {
		t.Error("the first rows are gone")
	}
	if tt.HasText("b249") {
		t.Error("the last branch is drawn although the list is capped")
	}
	clickInSidebar(t, tt, more)
	settle(tt, pump, 3)
	if !tt.HasText("b249") {
		t.Errorf("after Show more the last branch is still not drawn; texts: %q", tt.Texts())
	}
	if !tt.HasText("Show fewer") {
		t.Error("an opened list has no way back")
	}
	clickInSidebar(t, tt, "Show fewer")
	settle(tt, pump, 3)
	if tt.HasText("b249") {
		t.Error("Show fewer left the whole list drawn")
	}
}

func TestSearchDrawsEveryMatchWhateverTheCap(t *testing.T) {
	w, pump, sh := branchesShell(t, 250)
	sh.gitRefsFilter = "b"
	tt := newTester(w, 1280, 800, true)
	settle(tt, pump, 3)
	if !tt.HasText("b249") {
		t.Errorf("a search hid matches behind the cap; texts: %q", tt.Texts())
	}
	if rows := moreRows(tt.Texts()); len(rows) > 0 {
		t.Errorf("a search shows %q", rows)
	}
}

func TestAShortListGetsNoMoreRow(t *testing.T) {
	w, pump, sh := branchesShell(t, 20)
	_ = sh
	tt := newTester(w, 1280, 800, true)
	settle(tt, pump, 3)
	if rows := moreRows(tt.Texts()); len(rows) > 0 {
		t.Errorf("a short list shows %q", rows)
	}
	if !tt.HasText("b019") {
		t.Error("a short list lost a row")
	}
}

// A folder holds its children when open and counts as one row when shut.
func TestShutFoldersCountAsOneRow(t *testing.T) {
	root := gitFixture(t, "folders")
	head := gitRun(t, root, "rev-parse", "HEAD")
	for i := 0; i < 150; i++ {
		gitRun(t, root, "update-ref", fmt.Sprintf("refs/heads/team/t%03d", i), head)
	}
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	pump.until(t, "the references", func() bool { return sh.git.refsRead && len(sh.git.branches) >= 150 })
	sh.setGitFolded(sh.gitFoldKey("local/team"), true)
	tt := newTester(w, 1280, 800, true)
	settle(tt, pump, 3)
	if rows := moreRows(tt.Texts()); len(rows) > 0 {
		t.Errorf("a shut folder of 150 branches counted as %q; it is one row", rows)
	}
}
