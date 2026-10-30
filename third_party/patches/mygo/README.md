# MyGo 原生层与动画计时补丁

上游：`egoist/mygo`，修订 `558e498142b0e91b8e659f7e1f9aae5a455108f3`，MIT，许可见 [mygo.txt](../../licenses/mygo.txt)。不替换菜单或绘制应用自有控件；保留 MyGo / Windows 的菜单渲染和行为。

## 范围与原因

`windows-native.patch` 修改 `internal/windows/menu.go`、`menudraw.go`、`surface.go`、`webview_embed.go`，并在上游测试文件中增加回归。菜单部分的范围是 MyGo 自绘的全部弹出菜单项：托盘菜单、按钮菜单（如预设的“更多”菜单）、右键菜单和文本编辑菜单。

- **文案右缘被裁切（实测根因）**：Windows 会给每个自绘菜单项再加一段宽度，数值是系统菜单字体平均字宽的 2 倍（`GdiGetCharDimensions` 的算法）。原代码按 `SM_CXMENUCHECK - 1` 扣除。英文系统的菜单字体 Segoe UI 两者相等；中文系统的菜单字体 Microsoft YaHei UI 在 96 DPI 下 `SM_CXMENUCHECK` 为 17、平均字宽为 7，多扣 2 像素，菜单最宽一项的末字右缘被 `DrawText` 按文字区裁掉。现在按实测的系统菜单字体平均字宽扣除。
- **字形外伸被裁切**：YaHei UI 的部分字形比字宽多出 1 像素（左或右）。文字区两侧有 16 DIP 留白，现以 `DT_NOCLIP` 绘制，由菜单项自身的位图限定边界。
- **显示器 DPI**：菜单建在指针所在显示器，托盘 owner 是隐藏窗口，DPI 不代表菜单所在显示器。现在把创建时的 DPI 保存在菜单项中，度量与绘制读取同一值。
- **调整区域时视频区露白（实测根因）**：原生 UI 中的网页视图（播放器）经 DirectComposition 显示在画布子窗口之上，其下是该子窗口自己的位图；MyGo 对该窗口的擦除背景消息不做绘制，位图保持创建时的白色。快速拖动分隔条使播放器区域变大、网页尚未按新尺寸重绘时，这块白色露出。现在有网页视图时，擦除背景用窗口的背景色（OneSub 设为黑色）填充，并在创建网页图层和窗口尺寸变化时触发擦除；不额外叠加色块。
- **放大文字**：原菜单行固定 32 DIP，字体随系统辅助功能文字大小放大。现在取固定行高与“文字行高＋内边距”的较大值。

`tools/prepare_mygo.py` 校验五个上游文件 SHA-256，把锁定依赖复制到忽略目录（只复制内容、不删除旧副本）、应用补丁，再以生成的 `-modfile` 定向替换 MyGo。Go 不允许覆盖模块缓存的 overlay；三个统一构建/测试入口因此共用该生成模块文件，不修改模块缓存或已跟踪的 `go.mod/go.sum`，不跟踪完整上游源码副本。上游变化时哈希不符即停止，需重新评估补丁。

## Blocks 动画计时

`loading-clock.patch` 只为 `plugins/loading/loading.go` 增加可选 `StopDuration`：暂停时，计时速度从 1 平缓降至 0；按绝对时间积分，不随帧率改变最终相位；重新启动从可见相位接续。零值保留上游即时暂停语义。`spinners.go` 的 Blocks 几何、关键帧、默认周期和绘制完全不改，仍由 MyGo 原生插件渲染；OneSub 将单个实例放大至等待区域，再用 MyGo `AnimatePose` 延迟淡出。

上游 `plugins/loading/loading.go` SHA-256：`fe346a269c6110c054de510aa53aae625e0113b6efbab45e2216c6eb0fa081b9`。`tools/prepare_mygo.py` 同时验证该基线并应用两份补丁；`build/test.ps1` 执行插件全部上游测试及新增的渐停／恢复计时测试。`StopDuration` 不改变系统减少动画设置；背景退出不会阻塞任务结果。

## 验收边界

- `TestOneSubPopupItemWidth`：弹出真实菜单，用 `GetMenuItemRect` 读取菜单项宽度，要求不小于最宽文字所需宽度（旧算法实测 156 < 158 像素，失败；修复后通过）。
- `TestOneSubPopupFontBounds`：真实 GDI 字体，以 14/48/64 字号验证中文行高与留白。
- `TestOneSubPopupMonitorDPI`：96/144/192 DPI 下读取创建时的 DPI。
- 桌面实测：中文 Windows 11、96 DPI 单显示器下截取托盘菜单与预设菜单，文字墨迹与参考渲染逐列一致。多显示器混合 DPI 未实机验收。
