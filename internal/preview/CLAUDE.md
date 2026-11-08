# preview/
> L2 | 父级: ../../CLAUDE.md

Files 里 HTML 页面的"Open Preview"背后的本地预览：一个只绑定回环地址的静态服务和一个专用 WebView 窗口；外部导航一律转交系统浏览器。它加载的是回环地址上的页面，不需要应用内嵌前端，也没有页面桥。（Markdown / 图片的"富文本预览"需要内嵌的预览页，本应用没有，见 `internal/nativeui/files_webview.go` 的 `webPageShipped`。）

成员清单
- `preview.go`：项目作用域的预览窗口，只允许 localhost / 127.0.0.1 / [::1]，外部跳转拦截并交给系统浏览器；`IsLoopbackURL`、`NormalizePreviewTarget`。
- `static.go`：静态服务——只读、限定在给定根目录内、绑定 `127.0.0.1:0`（随机端口）、目录列表不含隐藏项、可跟随"显示被忽略文件"开关；`URLFor` 转义路径。
- `preview_test.go`：回环判定与预览目标规范化。
- `static_test.go`：只读服务根目录、目录列表无隐藏项、私有内容拒绝、被忽略开关、URL 转义。

[PROTOCOL]: 变更时更新此头部，然后检查 ../../CLAUDE.md
