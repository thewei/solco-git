package lsp

/**
 * [INPUT]: 用户机器上已装的语言服务器（PATH，外加 Finder 启动的应用看不到的常见目录）
 * [OUTPUT]: Spec 目录（语言 → 服务器命令、扩展名、languageId）、SpecFor（按文件名选）、Resolve/Detect（找得到哪些）、childEnv（给子进程补全 PATH）
 * [POS]: 只用已装的，不下载、不装、不管版本（2026-11-13 决定第 (4) 条）；找不到的语言安静回退为只有语法高亮
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Spec is one language server the app knows how to talk to.
type Spec struct {
	ID       string            // stable key, also the log's "language"
	Name     string            // what the status line and Settings call it
	Commands [][]string        // candidates in order of preference: argv[0] is looked up
	Exts     map[string]string // file extension (with dot) → LSP languageId
}

// Specs are the supported servers. A language is added here, with a test
// that it is found by SpecFor, and nowhere else.
func Specs() []Spec {
	return []Spec{
		{ID: "go", Name: "Go", Commands: [][]string{{"gopls"}},
			Exts: map[string]string{".go": "go"}},
		{ID: "typescript", Name: "TypeScript / JavaScript", Commands: [][]string{{"typescript-language-server", "--stdio"}},
			Exts: map[string]string{".ts": "typescript", ".mts": "typescript", ".cts": "typescript", ".tsx": "typescriptreact",
				".js": "javascript", ".mjs": "javascript", ".cjs": "javascript", ".jsx": "javascriptreact"}},
		{ID: "rust", Name: "Rust", Commands: [][]string{{"rust-analyzer"}},
			Exts: map[string]string{".rs": "rust"}},
		{ID: "python", Name: "Python", Commands: [][]string{{"basedpyright-langserver", "--stdio"}, {"pyright-langserver", "--stdio"}, {"pylsp"}},
			Exts: map[string]string{".py": "python", ".pyi": "python"}},
		{ID: "clangd", Name: "C / C++ / Objective-C", Commands: [][]string{{"clangd"}},
			Exts: map[string]string{".c": "c", ".h": "c", ".cc": "cpp", ".cpp": "cpp", ".cxx": "cpp", ".hpp": "cpp", ".hh": "cpp",
				".m": "objective-c", ".mm": "objective-cpp"}},
		{ID: "swift", Name: "Swift", Commands: [][]string{{"sourcekit-lsp"}},
			Exts: map[string]string{".swift": "swift"}},
		{ID: "zig", Name: "Zig", Commands: [][]string{{"zls"}},
			Exts: map[string]string{".zig": "zig"}},
	}
}

// SpecFor is the server for a file, by its extension, and the file's
// languageId.
func SpecFor(path string) (Spec, string, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	for _, sp := range Specs() {
		if id, ok := sp.Exts[ext]; ok {
			return sp, id, true
		}
	}
	return Spec{}, "", false
}

// extraDirs are where a Finder-launched app (which does not inherit the
// shell's PATH) will not look on its own and a developer's tools live.
func extraDirs() []string {
	home, _ := os.UserHomeDir()
	dirs := []string{"/opt/homebrew/bin", "/usr/local/bin", "/usr/local/go/bin", "/usr/bin"}
	if home != "" {
		dirs = append(dirs, filepath.Join(home, "go/bin"), filepath.Join(home, ".cargo/bin"), filepath.Join(home, ".local/bin"),
			filepath.Join(home, ".bun/bin"), filepath.Join(home, ".volta/bin"), filepath.Join(home, ".npm-global/bin"))
	}
	return dirs
}

// findExecutable looks name up on PATH, then in extraDirs. Empty when it is
// not installed.
func findExecutable(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	for _, d := range extraDirs() {
		p := filepath.Join(d, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return p
		}
	}
	return ""
}

// Resolve returns the command to run for the spec — the first candidate that
// is installed, with its absolute path in argv[0] — or nil.
func (sp Spec) Resolve() []string {
	for _, c := range sp.Commands {
		if p := findExecutable(c[0]); p != "" {
			return append([]string{p}, c[1:]...)
		}
	}
	return nil
}

// Detected is what Settings lists for a spec.
type Detected struct {
	Spec    Spec
	Command []string // nil: not installed
}

// Detect resolves every spec.
func Detect() []Detected {
	out := make([]Detected, 0, len(Specs()))
	for _, sp := range Specs() {
		out = append(out, Detected{Spec: sp, Command: sp.Resolve()})
	}
	return out
}

// childEnv is the app's environment with PATH widened by extraDirs, so a
// server started from Finder still finds `go`, `node` or `cargo`.
func childEnv() []string {
	env := os.Environ()
	have := map[string]bool{}
	path := ""
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		have[d] = true
		path += d + string(os.PathListSeparator)
	}
	for _, d := range extraDirs() {
		if !have[d] {
			path += d + string(os.PathListSeparator)
		}
	}
	path = strings.TrimSuffix(path, string(os.PathListSeparator))
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if !strings.HasPrefix(kv, "PATH=") {
			out = append(out, kv)
		}
	}
	return append(out, "PATH="+path)
}
