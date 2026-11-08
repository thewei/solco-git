package nativeui

/**
 * [INPUT]: files_ops.go（filesPathItems、filesQuickLook）、files_native_preview.go 各正文的右键菜单、Files 树
 * [OUTPUT]: 2026-11-08 Quick Look 回归（与 Solco 同一合同）：树行（文件有、文件夹没有）、Actions 菜单、代码视图、Markdown 渲染视图、SVG 图像视图的右键都有 Quick Look 且作用在当前文件上
 * [POS]: 只验证菜单接线（用 filesQuickLook 变量拦下真实的 qlmanage）；Quick Look 本身是系统的
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// watchQuickLook replaces the Quick Look launcher for the test and returns
// the paths it was asked to show.
func watchQuickLook(t *testing.T) *[]string {
	t.Helper()
	var shown []string
	prev := filesQuickLook
	filesQuickLook = func(path string) { shown = append(shown, path) }
	t.Cleanup(func() { filesQuickLook = prev })
	return &shown
}

// rightClickBody right-clicks in the middle of the preview's body, below the
// bar that names the file.
func rightClickBody(t *testing.T, tt *ui.Tester) []string {
	t.Helper()
	tt.RightClickAt(900, 500)
	menu := tt.Menu()
	if menu == nil {
		t.Fatal("the body has no context menu")
	}
	return menu
}

func TestFilesQuickLookIsOnEveryFileSurface(t *testing.T) {
	shown := watchQuickLook(t)
	_, pump, sh, tt, root := previewShell(t, map[string][]byte{
		"notes.md":        []byte("# Notes\n\nbody text\n"),
		"logo.svg":        []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="40" height="20"><rect width="40" height="20" fill="#c00"/></svg>`),
		"main.go":         []byte("package main\n\nfunc main() {}\n"),
		"docs/readme.txt": []byte("x\n"),
	})
	notes, logo, code := filepath.Join(root, "notes.md"), filepath.Join(root, "logo.svg"), filepath.Join(root, "main.go")
	sh.refreshFiles()
	settle(tt, pump, 4)

	// The tree: a file's row has it, a folder's does not.
	if err := tt.RightClick("notes.md"); err != nil {
		t.Fatalf("no tree row to right-click: %v (%q)", err, tt.Texts())
	}
	menu := tt.Menu()
	for _, want := range []string{"Quick Look", "Copy Path", "Reveal in Finder", "Rename…", "Delete…"} {
		if !slices.Contains(menu, want) {
			t.Fatalf("a file row lost %q: %q", want, menu)
		}
	}
	if slices.Index(menu, "Quick Look") > slices.Index(menu, "Copy Path") {
		t.Fatalf("Quick Look comes first among the helpers: %q", menu)
	}
	if err := tt.ChooseMenuItem("Quick Look"); err != nil {
		t.Fatal(err)
	}
	if len(*shown) != 1 || (*shown)[0] != notes {
		t.Fatalf("Quick Look must show the row's file, got %q", *shown)
	}
	if err := tt.RightClick("docs"); err != nil {
		t.Fatalf("no folder row: %v", err)
	}
	if menu := tt.Menu(); slices.Contains(menu, "Quick Look") || !slices.Contains(menu, "Reveal in Finder") || !slices.Contains(menu, "New File…") {
		t.Fatalf("a folder has nothing to look at, only the path helpers and its own actions: %q", menu)
	}
	tt.CloseMenu()

	for name, tc := range map[string]struct {
		path  string
		extra []string
	}{
		"Markdown, rendered": {notes, []string{"Copy", "Select All"}},
		"SVG, drawing":       {logo, nil},
		"code":               {code, nil},
	} {
		*shown = nil
		openPreview(t, sh, pump, tt, tc.path)
		menu := rightClickBody(t, tt)
		for _, want := range append([]string{"Quick Look", "Copy Path", "Reveal in Finder"}, tc.extra...) {
			if !slices.Contains(menu, want) {
				t.Errorf("%s: the body's menu lost %q: %q", name, want, menu)
			}
		}
		if err := tt.ChooseMenuItem("Quick Look"); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(*shown) != 1 || (*shown)[0] != tc.path {
			t.Errorf("%s: Quick Look must show the open file %q, got %q", name, tc.path, *shown)
		}
	}

	// The title bar's Actions menu carries the open file's helpers too, ahead
	// of the tree-wide actions.
	*shown = nil
	openPreview(t, sh, pump, tt, code)
	if err := tt.Click("Actions"); err != nil {
		t.Fatalf("the Files view has no Actions menu: %v", err)
	}
	menu = tt.Menu()
	for _, want := range []string{"Quick Look", "Copy Path", "Reveal in Finder", "New File…"} {
		if !slices.Contains(menu, want) {
			t.Fatalf("the Actions menu lost %q: %q", want, menu)
		}
	}
	if err := tt.ChooseMenuItem("Quick Look"); err != nil {
		t.Fatal(err)
	}
	if len(*shown) != 1 || (*shown)[0] != code {
		t.Fatalf("Actions → Quick Look must show the open file %q, got %q", code, *shown)
	}
}

// TestImagePreviewKeepsItsContextMenu: a decoded image is a GPU bitmap, which a
// headless window cannot hold, so its card cannot be right-clicked here; the
// wiring is pinned in the source instead — the image's body carries the same
// file menu (Quick Look, Copy Path, Reveal in Finder) as the other previews.
func TestImagePreviewKeepsItsContextMenu(t *testing.T) {
	src, err := os.ReadFile("files_native_preview.go")
	if err != nil {
		t.Fatal(err)
	}
	_, after, ok := strings.Cut(string(src), "case st.nativeImage != nil:")
	if !ok {
		t.Fatal("the image's case is gone from nativePreviewBody; update this guard with it")
	}
	body, _, _ := strings.Cut(after, "case st.webImage:")
	if !strings.Contains(body, "ContextMenu(s.previewFileMenu)") {
		t.Errorf("the image preview lost its file menu: %.200q", body)
	}
}
