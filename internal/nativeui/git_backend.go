package nativeui

import (
	"os"
	"sync"

	"github.com/thewei/solco-gitkit/engine"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

/**
 * [INPUT]: solco-gitkit/engine（LazyGit 命令层适配，嵌套模块）、环境变量 SOLCOGIT_GIT_BACKEND
 * [OUTPUT]: gitBackend()——装到每个 gitworkbench.Runner.Backend 的那一个 Engine；SOLCOGIT_GIT_BACKEND=argv 时为 nil（全部回到 argv 版 git）
 * [POS]: Shell 一侧唯一决定「哪些 Git 能力由 LazyGit 提供」的接缝；能力清单在 engine/CLAUDE.md，不在这里
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

// sharedEngine is one Engine for the whole app: it keeps a LazyGit session
// per repository root, so every workspace's service reuses the same ones.
var sharedEngine = sync.OnceValue(func() gitworkbench.Backend {
	e := engine.New()
	e.Commands = sharedCommandLog()
	return e
})

// gitBackend is the Backend a Runner here is given. SOLCOGIT_GIT_BACKEND=argv
// is the way back to the plain `git` path (a debugging and rollback switch,
// not a setting).
func gitBackend() gitworkbench.Backend {
	if os.Getenv("SOLCOGIT_GIT_BACKEND") == "argv" {
		return nil
	}
	return sharedEngine()
}
