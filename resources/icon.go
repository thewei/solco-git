/*
 * [INPUT]: 依赖同目录 icon.png 的 Solco Git 应用图稿。
 * [OUTPUT]: 导出 AppIconPNG，供欢迎页复用应用图标。
 * [POS]: 资源包连接 MyGo 打包输入与原生窗口展示。
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */
package resources

import _ "embed"

//go:embed icon.png
var AppIconPNG []byte
