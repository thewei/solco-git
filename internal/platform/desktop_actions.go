package platform

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"time"
)

// DesktopActionTimeout bounds short-lived desktop shell helper operations.
const DesktopActionTimeout = 3 * time.Second

// QuickLook shows path in macOS Quick Look (qlmanage's preview panel, closed
// with Escape). The viewer is a short-lived child that owns nothing of ours, so
// it is started and left to exit on its own.
func QuickLook(path string) error {
	if runtime.GOOS != "darwin" {
		return errors.ErrUnsupported
	}
	cmd := exec.Command("/usr/bin/qlmanage", "-p", path)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// OpenFile opens the file with the default editor or application. MyGo's
// Shell.OpenPath opens a file with the application of its type; macOS's
// `open -t` is the text editor, which the framework has no call for.
func OpenFile(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), DesktopActionTimeout)
	defer cancel()

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", "-t", path)
	case "windows":
		cmd = exec.CommandContext(ctx, "cmd", "/c", "start", "", path)
	default:
		cmd = exec.CommandContext(ctx, "xdg-open", path)
	}
	return cmd.Run()
}
