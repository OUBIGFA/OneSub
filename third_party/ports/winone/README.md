# WinOne 浅色主题配色复用

## 来源

- 项目：[OUBIGFA/WinOne](https://github.com/OUBIGFA/WinOne)。
- 固定修订：`90c479ce28b0cc6d690d3fa35115ea7dd4a4c89f`。
- 原文件：[`apps/desktop-mygo/kit/theme.go`](https://github.com/OUBIGFA/WinOne/blob/90c479ce28b0cc6d690d3fa35115ea7dd4a4c89f/apps/desktop-mygo/kit/theme.go)。
- 原文件 SHA-256：`0e954fc1899684d0cae903f2db18e4bcc30467b3c6009a2b8f440fa581d6ee16`；5,264 字节。
- 许可：GPL-3.0-only，依据仓库根目录的 GPL v3 文本；原许可副本见 [`../../licenses/winone.txt`](../../licenses/winone.txt)。原许可 SHA-256：`3972dc9744f6499f0f9b2dbf76696f2ae7ad8af9b23dde66d6af86c9dfb36986`。
- 来源归属：WinOne 项目贡献者。不是 MyGo 上游的默认主题。

## 复用边界

2026-11-01 将原文件的 `lightTheme` 中性背景、控件、文字、边框、状态色及 `lightPalette` 中实际使用的面板、侧栏、分隔线和警告浅底色适配到 `apps/desktop/theme_winone.go`。强调色明确保留 OneSub 原有的 MyGo 蓝色 `#2563eb`，悬停为 `#1d4ed8`、按下为 `#1e40af`；文本选择、焦点与功能选中浅底同步使用蓝色。保留 WinOne 的高对比度边框/文字混色系数。

只复用配色数据，不引入 WinOne 的运行时、界面代码、绘制逻辑、资源、加载动画或独立控件体系。所有控件继续使用 OneSub 固定版本的 MyGo 原生 UI；字号、间距、布局与深色主题保持 OneSub 的约定。

该适配部分保留 GPL v3 来源；与 OneSub AGPL v3 部分组合时遵循 GPL v3 第 13 条的组合许可条件，不将来源改称 MIT。安装器包含许可副本；后续公开分发仍受项目既有源码材料和发布门禁约束。

## 验证

`apps/desktop/theme_winone_test.go` 覆盖浅色颜色值、面板角色、高对比度及深色不变；工作台与设置页的中英文、浅深色原生渲染回归覆盖实际消费者。颜色值测试不替代真实显示器与读屏验收。
