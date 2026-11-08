// Solco Git — a Git client and Files viewer on MyGo Native UI: one window, a
// workspace per repository (a pull-down in the title band switches them). It
// reads and changes repositories through the git CLI only; there is no Herdr,
// Pane or terminal in it.
package main

import (
	"log/slog"
	"os"
	"runtime"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	updaterNative "github.com/egoist/mygo/plugins/updater/native"
	"github.com/egoist/mygo/ui"

	"github.com/thewei/solco-git/internal/applog"
	"github.com/thewei/solco-git/internal/nativeui"
	"github.com/thewei/solco-git/internal/settings"
)

// devAppName keeps an unpackaged run's data and logs apart from the installed
// app's, and gives MyGo's temporary build binaries one stable name.
const devAppName = "Solco-Git-dev"

func main() {
	if !mygo.App.IsPackaged() {
		mygo.App.SetName(devAppName)
	}

	// `mygo generate` runs the app just far enough to write the TypeScript
	// client; it must reach App.Run even while the real app holds the lock.
	generating := os.Getenv("MYGO_GENERATE") != ""
	if !generating && !mygo.App.RequestSingleInstanceLock() {
		return
	}

	// The updater window is drawn in native UI. Without an `updates` block in
	// mygo.config.ts it truthfully reports that this build cannot check; it
	// goes live once signed releases are published (docs/releasing.md).
	mygo.Use(updaterNative.Plugin)

	userDataDir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		slog.Error("resolve user data dir", "error", err)
		os.Exit(1)
	}
	logsDir := mustLogsDir(userDataDir)
	if _, err := applog.Setup(logsDir); err != nil {
		slog.Error("setup application logging", "error", err)
	} else if stop, err := applog.CaptureStderr(logsDir); err != nil {
		slog.Warn("capture stderr failed; runtime panics stay invisible", "error", err)
	} else {
		defer stop()
	}

	window := nativeui.NewWindow(settings.NewFileStore(userDataDir))
	window.SetUpdateItem(updater.MenuItem)

	var raiseMain func()
	raise := func() {
		if raiseMain != nil {
			raiseMain()
		}
	}

	// solcogit://open?repo=&view=&file= — Solco's link into this app, and
	// `open -a "Solco Git" <folder>`. A link that arrives before the window
	// exists waits inside the Window; neither is ever logged.
	mygo.App.OnOpenURL(func(raw string) {
		window.OpenURL(raw)
		raise()
	})
	mygo.App.OnOpenFile(func(path string) {
		window.OpenPath(path)
		raise()
	})

	var closeOnce sync.Once
	closeWindow := func() { closeOnce.Do(window.Close) }

	// The close button only hides the window (its workspaces stay); a real quit
	// comes from the app menu (RoleQuit → App.Quit), which fires OnBeforeQuit
	// before closing windows. The flag tells OnClose the two apart.
	quitting := false
	mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) { quitting = true })

	mygo.App.WhenReady(func() {
		slog.Info("application ready", "log_path", applog.Path())
		opts := mygo.WindowOptions{
			Title:  "Solco Git",
			Width:  1280,
			Height: 800,
			// Sidebar + a usable content column: below this the repository
			// bar and diff gutters squeeze into unreadability.
			MinWidth:  900,
			MinHeight: 560,
			// The last bounds (position, size, maximized) come back unless
			// Settings turned restoration off.
			StateKey: window.WindowStateKey(),
			// The sidebar runs under the traffic lights and the title band
			// draws itself beside them: hidden title bar, lights at the
			// band's height, and a base color matching the content canvas so
			// the first frame does not flash.
			TitleBarStyle:        mygo.TitleBarHidden,
			TrafficLightPosition: &mygo.Point{X: 16, Y: 15},
			BackgroundColor:      "light-dark(#fcfcfc, #0a0a0a)",
			Content:              ui.View(window.View),
		}
		if runtime.GOOS == "darwin" {
			// The sidebar is the system's translucent material: it shows
			// wherever the view draws no background.
			opts.Vibrancy = mygo.VibrancySidebar
		}
		win := mygo.NewWindow(opts)
		window.AttachWindow(win)
		win.OnClose(func(e *mygo.CloseEvent) {
			if !quitting {
				window.SaveState()
				e.PreventDefault()
				win.Hide()
				return
			}
			closeWindow()
		})
		// Re-open surfaces: Dock-icon clicks and a second launch both unhide
		// the window.
		showMain := func() {
			if !win.IsVisible() {
				win.Show()
			}
			win.Focus()
		}
		raiseMain = showMain
		window.SetRaise(showMain)
		mygo.App.OnActivate(func(hasVisibleWindows bool) { showMain() })
		mygo.App.OnSecondInstance(func(args []string, workingDir string) { showMain() })
		slog.Info("main window shown", "bounds", win.Bounds())
	})

	if err := mygo.App.Run(); err != nil {
		slog.Error("application run", "error", err)
	}
	closeWindow()
	if err := applog.Close(); err != nil {
		slog.Warn("close application log", "error", err)
	}
}

// mustLogsDir resolves the platform log directory (~/Library/Logs/Solco Git on
// macOS), falling back to a folder next to the user data.
func mustLogsDir(userDataDir string) string {
	if dir, err := mygo.App.Path(mygo.PathLogs); err == nil && dir != "" {
		return dir
	}
	slog.Warn("resolve log dir failed; logging next to the user data")
	if userDataDir == "" {
		return os.TempDir()
	}
	return userDataDir + string(os.PathSeparator) + "logs"
}
