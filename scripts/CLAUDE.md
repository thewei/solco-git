# scripts/
> L2 | 父级: ../CLAUDE.md

成员清单
- `kit-parity.sh`：只在本机用——UI kit 复制自 Solco 的 `internal/nativeui`，此脚本逐个同名文件报告"完全一致 / 有差异（改动行数）"，`--all` 连一致的也列出，`--diff <file.go>` 看单个文件的完整差异；需要同级的 `../solco`（别处用 `SOLCO_DIR`），CI 没有，所以不进 CI。

[PROTOCOL]: 变更时更新此头部，然后检查 ../CLAUDE.md
