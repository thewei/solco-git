package nativeui

/**
 * [INPUT]: files_markdown_test.go 的 previewShell / writeAt（真实临时仓库上的 Shell）
 * [OUTPUT]: filesOpsTestShell、writePreviewFile——与 ../solco 同名的夹具，让 files_markdown_{rows,find,rich,html}_test.go 两个 App 里逐字相同
 * [POS]: 这些测试只在自己的 ui.Tester 里渲染 Markdown 正文，Shell 只提供菜单与剪贴板；夹具名与 Solco 一致，仅此而已
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// filesOpsTestShell is a Shell on a fresh repository, the tester of its window
// and the repository's root: where Solco's tests open its Files panel, these
// use the Files view.
func filesOpsTestShell(t *testing.T) (*Shell, *ui.Tester, string) {
	t.Helper()
	_, _, sh, tt, root := previewShell(t, nil)
	return sh, tt, root
}

// writePreviewFile writes data under root and returns its path.
func writePreviewFile(t *testing.T, root, name string, data []byte) string {
	t.Helper()
	return writeAt(t, root, name, data)
}
