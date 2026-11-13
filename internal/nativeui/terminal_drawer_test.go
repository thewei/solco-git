package nativeui

/**
 * [INPUT]: terminal_drawer.go、真实临时仓库、MyGo 终端插件（libghostty-vt 取不到时整组跳过）、headless ui.Tester
 * [OUTPUT]: 终端抽屉的合同（2026-11-13 决定第 (5) 条）：⌃` 的开关起一个 shell，工作目录 = 仓库根，包装层写下的 pid 就是 shell 的 pid（所以"有子进程"= 有进程在跑）；没有进程在跑时 Close 立即结束，有进程时先问（Cancel 什么都不动，确认才杀进程）；有进程时移除工作区先问、没有就直接移除；shell 自己退出后抽屉头显示退出码
 * [POS]: 用 SHELL=/bin/sh 保证确定性；每条规则做过变异检查（去掉询问分支 / 去掉 pid 文件 / 不设 Dir，对应测试必红）
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

func termShell(t *testing.T) (*Window, *uiPump, *Shell, *ui.Tester, string) {
	t.Helper()
	if err := terminal.Load(); err != nil {
		t.Skip("terminal library unavailable: ", err)
	}
	t.Setenv("SHELL", "/bin/sh")
	w, pump, sh, tt, root := previewShell(t, nil)
	sh.showView(viewChanges)
	settle(tt, pump, 2)
	sh.toggleTerminal()
	settle(tt, pump, 3)
	t.Cleanup(sh.closeTerminal)
	return w, pump, sh, tt, root
}

// waitFile pumps until the shell has written path.
func waitFile(t *testing.T, pump *uiPump, tt *ui.Tester, path string) string {
	t.Helper()
	pump.until(t, "the shell to write "+filepath.Base(path), func() bool {
		data, err := os.ReadFile(path)
		return err == nil && strings.HasSuffix(string(data), "\n")
	})
	data, _ := os.ReadFile(path)
	return strings.TrimSpace(string(data))
}

func TestTerminalRunsTheShellInTheRepositoryRootAndItsPidIsTheShells(t *testing.T) {
	_, pump, sh, tt, root := termShell(t)
	d := &sh.term
	if !d.open || d.t == nil {
		t.Fatalf("⌃` opens the drawer and starts a shell: open=%v term=%v", d.open, d.t != nil)
	}
	out := filepath.Join(t.TempDir(), "out")
	d.t.Send([]byte("pwd > " + out + ".pwd; echo $$ > " + out + ".pid\n"))
	if got := waitFile(t, pump, tt, out+".pwd"); got != root {
		t.Errorf("the shell starts in the repository root: %q, want %q", got, root)
	}
	shellPid := waitFile(t, pump, tt, out+".pid")
	pidFile, _ := os.ReadFile(d.pidFile)
	if string(pidFile) != shellPid {
		t.Errorf("the recorded pid is the shell's: file=%q shell=%q", pidFile, shellPid)
	}
	saveEvidence(t, "terminal-drawer", tt)
	sh.toggleTerminal()
	if d.open || d.t == nil {
		t.Errorf("a second ⌃` hides the drawer and keeps the shell: open=%v term=%v", d.open, d.t != nil)
	}
}

// runSleep starts a long foreground command and waits until the shell has it.
func runSleep(t *testing.T, pump *uiPump, sh *Shell, seconds string) {
	t.Helper()
	sh.term.t.Send([]byte("sleep " + seconds + "\n"))
	pump.until(t, "the process to run", sh.termBusy)
}

func sleeping(seconds string) bool {
	return exec.Command("pgrep", "-f", "sleep "+seconds).Run() == nil
}

func TestClosingTheTerminalAsksOnlyWhileAProcessRuns(t *testing.T) {
	_, pump, sh, tt, _ := termShell(t)
	d := &sh.term
	pump.until(t, "the shell to be up", func() bool { return d.pidFile != "" && fileHas(d.pidFile) })

	sh.askTerminal("close") // nothing runs: it ends at once
	if d.t != nil || d.open || d.ask != "" {
		t.Fatalf("an idle terminal closes without asking: term=%v open=%v ask=%q", d.t != nil, d.open, d.ask)
	}

	sh.toggleTerminal()
	settle(tt, pump, 3)
	pump.until(t, "the new shell", func() bool { return fileHas(d.pidFile) })
	runSleep(t, pump, sh, "31.71")
	sh.askTerminal("close")
	settle(tt, pump, 3)
	if d.ask != "close" || !tt.HasText("Close the terminal?") {
		t.Fatalf("a running process raises the sheet: ask=%q texts=%q", d.ask, tt.Texts())
	}
	saveEvidence(t, "terminal-close-ask", tt)
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 2)
	if d.ask != "" || d.t == nil || !sleeping("31.71") {
		t.Fatalf("Cancel changes nothing: ask=%q term=%v sleeping=%v", d.ask, d.t != nil, sleeping("31.71"))
	}

	sh.askTerminal("close")
	settle(tt, pump, 3)
	if err := tt.Click("Close Terminal"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 2)
	if d.t != nil || d.open {
		t.Fatalf("Close Terminal ends it: term=%v open=%v", d.t != nil, d.open)
	}
	deadline := time.Now().Add(5 * time.Second)
	for sleeping("31.71") && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if sleeping("31.71") {
		t.Error("the process outlived its terminal")
	}
}

func fileHas(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	_, perr := strconv.Atoi(strings.TrimSpace(string(data)))
	return perr == nil
}

func TestRemovingAWorkspaceAsksWhileItsTerminalRuns(t *testing.T) {
	w, pump, sh, tt, _ := termShell(t)
	d := &sh.term
	pump.until(t, "the shell to be up", func() bool { return fileHas(d.pidFile) })

	w.requestRemoveWorkspace(w.indexOfID(w.workspaces[w.active].id))
	if w.removeAsk != 0 || len(w.workspaces) != 0 {
		t.Fatalf("an idle terminal does not hold the removal: ask=%d workspaces=%d", w.removeAsk, len(w.workspaces))
	}
	if d.t != nil {
		t.Error("removing the workspace ends its terminal")
	}

	root := gitFixture(t, "busy")
	sh2 := openAndLoad(t, w, pump, root)
	sh2.toggleTerminal()
	settle(tt, pump, 3)
	t.Cleanup(sh2.closeTerminal)
	pump.until(t, "the shell to be up", func() bool { return fileHas(sh2.term.pidFile) })
	runSleep(t, pump, sh2, "31.72")
	w.requestRemoveWorkspace(w.indexOfID(w.workspaces[w.active].id))
	if w.removeAsk == 0 || len(w.workspaces) != 1 {
		t.Fatalf("a running process holds the removal: ask=%d workspaces=%d", w.removeAsk, len(w.workspaces))
	}
}

func TestTerminalShowsTheExitCodeWhenTheShellEnds(t *testing.T) {
	_, pump, sh, tt, _ := termShell(t)
	d := &sh.term
	pump.until(t, "the shell to be up", func() bool { return fileHas(d.pidFile) })
	d.t.Send([]byte("exit 7\n"))
	pump.until(t, "the exit", func() bool { return d.exited })
	settle(tt, pump, 2)
	if d.exitCode != 7 || !tt.HasText("exited 7") {
		t.Errorf("the head says how it ended: code=%d texts=%q", d.exitCode, tt.Texts())
	}
}
