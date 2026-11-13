package platform

import (
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	// SolcoBundleName is Solco's bundle directory name.
	SolcoBundleName = "Solco.app"
	// SolcoEnvApp names one bundle explicitly — a development build, a
	// sandbox's copy — and wins over the standard places.
	SolcoEnvApp = "SOLCO_APP"
	// solcoMaxLink bounds the link, as Solco's own parser does.
	solcoMaxLink = 8192

	solcoRecheckEvery = 5 * time.Second
	solcoOpenTimeout  = 5 * time.Second
)

// ErrSolcoNotInstalled: Solco is not on this machine (or this is not macOS).
var ErrSolcoNotInstalled = errors.New("Solco is not installed")

// ErrSolcoBadFolder: the folder cannot be put in a link.
var ErrSolcoBadFolder = errors.New("nothing to open in Solco")

// SolcoTerminalURL is the link that asks Solco for a terminal in folder: Solco
// shows the Workspace rooted there, or creates one. The folder is the only
// thing it carries — no command (Solco never reads one). ok is false when
// folder cannot be put in a link.
func SolcoTerminalURL(folder string) (link string, ok bool) {
	if !filepath.IsAbs(folder) || strings.ContainsRune(folder, 0) {
		return "", false
	}
	query := url.Values{}
	query.Set("cwd", filepath.Clean(folder))
	link = "solco://terminal?" + query.Encode()
	if len(link) > solcoMaxLink {
		return "", false
	}
	return link, true
}

// SolcoLink is the one-way link from this app to Solco (never the other way,
// and nothing is imported from it): it finds Solco on this machine and asks
// it for a terminal. Where the folder goes is never logged.
type SolcoLink struct {
	mu        sync.Mutex
	checkedAt time.Time
	found     string

	// Seams for tests.
	goos   string
	now    func() time.Time
	stat   func(string) (os.FileInfo, error)
	getenv func(string) string
	home   func() (string, error)
	run    func(appPath, link string) error
}

// NewSolcoLink is the production link.
func NewSolcoLink() *SolcoLink {
	return &SolcoLink{goos: runtime.GOOS, now: time.Now, stat: os.Stat, getenv: os.Getenv, home: os.UserHomeDir, run: openSolco}
}

// NewSolcoLinkAt is a link to the bundle at path, handing links to run — for
// tests of the code that uses a link, which must not start Solco.
func NewSolcoLinkAt(path string, run func(appPath, link string) error) *SolcoLink {
	l := NewSolcoLink()
	l.goos = "darwin"
	l.stat = func(p string) (os.FileInfo, error) {
		if p == path && path != "" {
			return dirInfo{}, nil
		}
		return nil, os.ErrNotExist
	}
	l.getenv = func(k string) string {
		if k == SolcoEnvApp {
			return path
		}
		return ""
	}
	l.home = func() (string, error) { return "", os.ErrNotExist }
	l.run = run
	return l
}

// dirInfo reports a directory for NewSolcoLinkAt's stand-in file system.
type dirInfo struct{ os.FileInfo }

func (dirInfo) IsDir() bool { return true }

// Path is Solco's bundle: the SOLCO_APP override, then /Applications, then
// ~/Applications; "" when it is not found. The answer is cached a few seconds,
// because the UI asks every frame.
func (l *SolcoLink) Path() string {
	if l == nil {
		return ""
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.goos != "darwin" {
		return ""
	}
	if now := l.now(); l.checkedAt.IsZero() || now.Sub(l.checkedAt) >= solcoRecheckEvery {
		l.checkedAt, l.found = now, l.search()
	}
	return l.found
}

func (l *SolcoLink) search() string {
	var candidates []string
	if explicit := l.getenv(SolcoEnvApp); explicit != "" {
		candidates = append(candidates, explicit)
	}
	candidates = append(candidates, filepath.Join("/Applications", SolcoBundleName))
	if home, err := l.home(); err == nil && home != "" {
		candidates = append(candidates, filepath.Join(home, "Applications", SolcoBundleName))
	}
	for _, path := range candidates {
		if info, err := l.stat(path); err == nil && info.IsDir() && strings.HasSuffix(path, ".app") {
			return path
		}
	}
	return ""
}

// Available reports whether Solco is on this machine.
func (l *SolcoLink) Available() bool { return l.Path() != "" }

// OpenTerminal asks Solco for a terminal in folder. It returns once macOS has
// handed the link over, not when Solco has shown it.
func (l *SolcoLink) OpenTerminal(folder string) error {
	app := l.Path()
	if app == "" {
		return ErrSolcoNotInstalled
	}
	link, ok := SolcoTerminalURL(folder)
	if !ok {
		return ErrSolcoBadFolder
	}
	return l.run(app, link)
}

// openSolco is `open -a <bundle> <link>`: argv only (no shell), and the link
// goes to exactly that bundle, so a second copy that also registered the
// scheme is not picked by accident.
func openSolco(appPath, link string) error {
	ctx, cancel := context.WithTimeout(context.Background(), solcoOpenTimeout)
	defer cancel()
	return exec.CommandContext(ctx, "open", "-a", appPath, link).Run()
}
