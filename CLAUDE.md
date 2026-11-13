# Solco Git — native macOS Git client + Files viewer (Go / MyGo Native UI)

Go 1.27.3 + MyGo Native UI (upstream v0.8.1, no vendored fork). One window, a workspace per repository chosen from a pull-down in the sidebar's foot (in the title band while the sidebar is shut), with Back / Forward arrows on the sidebar band as in Solco (a Solco Git workspace is this app's own list of repositories — nothing to do with Herdr's); Git only through `git` (the runner in `solco-gitkit`). The standalone form of Solco's former Git and Files views (decision: Solco `docs/product-decisions.md`, 2026-11-07 · Solco Git and the right panel).

<directory>
main.go - app entry: first `engine.RunDaemon()` (Git runs this program as its editor while the LazyGit engine rewrites history; it does that one job and exits), single-instance lock, updater plugin (Check for Updates…), applog, the one Window, `solcogit://` and open-file handlers, hide-on-close / real quit
internal/nativeui/ - presentation only: Window (workspace pull-down, Back/Forward history, welcome, Quick Launch, menu bar, settings incl. workspace management, pending links), one Shell per workspace (Changes / Commits / Files), the UI kit copied from Solco (theme, components, icons, Git review, Files), headless tests with real temporary repositories (see internal/nativeui/CLAUDE.md)
internal/settings/ - the app's own preference file: appearance/density switches, a few workbench cosmetics, the window's restorable workspaces and recents; atomic 0600 write, newer schema refused, no credentials or content
internal/preview/ - loopback-only static server and WebView window behind a Files HTML page's "Open Preview"
internal/codehl/ - token classes and palettes the diff review colors code with (MyGo's own tokenizers)
internal/platform/ - bounded desktop helpers (open a file in the default editor; the one-way link to Solco: `solco://terminal?cwd=`)
internal/lsp/ - language-server client for the Files editor (read capabilities: diagnostics, hover, go to definition): only servers already installed, one process per language per workspace, JSON-RPC on `sourcegraph/jsonrpc2`, reconcile-style document sync; no UI, no solco import (see internal/lsp/CLAUDE.md)
internal/applog/ - size-bounded structured logging (2 MiB active + 3 backups) and stderr capture
scripts/ - kit-parity.sh: how far the UI kit has drifted from the sibling `../solco` (local only)
resources/ - the 1024 × 1024 app icon used by MyGo packaging and embedded in the welcome page
site/ - the GitHub Pages landing site (static HTML/CSS, no build step; https://thewei.github.io/solco-git/)
docs/ - [private: link into ../solco-docs] releasing.md: the updater key/secret setup (done) and the routine tag-and-publish steps
.github/workflows/ - checks.yml (vet + test + unsigned build + bundle structure on macOS), release.yml (tag → draft release with signed update manifests; stops early when the updater key is not set up) and pages.yml (deploys site/)
</directory>

<config>
go.mod - module github.com/thewei/solco-git, Go 1.27.3, MyGo v0.8.1, `solco-gitkit` at a tagged version (never a relative `replace`)
go.work - local and gitignored: makes `../solco-gitkit` win on a machine with the repositories side by side (see README.md)
mygo.config.ts - app name/identifier/version, the `solcogit` URL scheme, icon, macOS minimum and the `updates` block (public key + `thewei/solco-git`; the secret key is only in the CI secret and the maintainer's key directory — docs/releasing.md)
.gitignore - build output, the local go.work
</config>

Product rule: **Solco Git is a Git client with an in-place Files editor, not a workspace.** It owns native presentation, its own preferences and window state. Git truth stays in the repository and in `solco-gitkit`; there is no Workspace, Pane, agent, Herdr, account, cloud service or telemetry here, and nothing from the `solco` repository is imported. Since the 2026-11-13 decision (Solco `docs/product-decisions.md`, "Solco Git edits in place") the Files page is the one place that may write: files open read-only and a pencil button turns editing on, with fzf-style quick open (⌘P), a language-server client (servers already on PATH only) and a terminal drawer running the user's shell (or handing the repository to Solco, by setting) — each arriving in steps E1–E5; until a step lands, that part stays as described below. The only processes this app owns are that shell and the language servers, both ending with the workspace.

Implementation rule: do not hand-roll what a mature owner has. Search in order: `git` through `solco-gitkit` → MyGo native components/official plugins → a proven compatible implementation → custom code. A missing Git capability is added to `solco-gitkit` (with a real-repository test), never as a second way of running `git` here.

Private-assets rule: **code, tests and the rules an agent obeys live here; the development record (the release runbook, Solco's decisions and designs, keys) lives in the private `../solco-docs`, linked in at the same paths.** Read through the link; write and commit there (`commit-docs.sh` of the `solco-docs` skill in the Solco repository), never copy it into this repository or its commit messages; every commit is authored as the user's GitHub identity. Details: `AGENTS.md` "Private assets (solco-docs)".

[PROTOCOL]: 变更时更新此头部，然后检查子目录 CLAUDE.md
