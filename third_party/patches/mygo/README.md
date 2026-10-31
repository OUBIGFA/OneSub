# MyGo 原生 UI 与 Windows 层补丁

上游：`egoist/mygo`，修订 `558e498142b0e91b8e659f7e1f9aae5a455108f3`，MIT，许可见 [mygo.txt](../../licenses/mygo.txt)。不替换菜单或绘制应用自有控件；保留 MyGo / Windows 的菜单渲染和行为。

## 范围与原因

`windows-native.patch` 修改 `internal/windows/menu.go`、`menudraw.go`、`surface.go`、`webview_embed.go`，并在上游测试文件中增加回归。菜单部分的范围是 MyGo 自绘的全部弹出菜单项：托盘菜单、按钮菜单（如预设的“更多”菜单）、右键菜单和文本编辑菜单。

- **文案右缘被裁切（实测根因）**：Windows 会给每个自绘菜单项再加一段宽度，数值是系统菜单字体平均字宽的 2 倍（`GdiGetCharDimensions` 的算法）。原代码按 `SM_CXMENUCHECK - 1` 扣除。英文系统的菜单字体 Segoe UI 两者相等；中文系统的菜单字体 Microsoft YaHei UI 在 96 DPI 下 `SM_CXMENUCHECK` 为 17、平均字宽为 7，多扣 2 像素，菜单最宽一项的末字右缘被 `DrawText` 按文字区裁掉。现在按实测的系统菜单字体平均字宽扣除。
- **字形外伸被裁切**：YaHei UI 的部分字形比字宽多出 1 像素（左或右）。文字区两侧有 16 DIP 留白，现以 `DT_NOCLIP` 绘制，由菜单项自身的位图限定边界。
- **显示器 DPI**：菜单建在指针所在显示器，托盘 owner 是隐藏窗口，DPI 不代表菜单所在显示器。现在把创建时的 DPI 保存在菜单项中，度量与绘制读取同一值。
- **调整区域时视频区露白（实测根因）**：原生 UI 中的网页视图（播放器）经 DirectComposition 显示在画布子窗口之上，其下是该子窗口自己的位图；MyGo 对该窗口的擦除背景消息不做绘制，位图保持创建时的白色。快速拖动分隔条使播放器区域变大、网页尚未按新尺寸重绘时，这块白色露出。现在有网页视图时，擦除背景用窗口的背景色（OneSub 设为黑色）填充，并在创建网页图层和窗口尺寸变化时触发擦除；不额外叠加色块。
- **放大文字**：原菜单行固定 32 DIP，字体随系统辅助功能文字大小放大。现在取固定行高与“文字行高＋内边距”的较大值。

`ui-interaction.patch` 复用固定版本已有的单行文字塑形和滚动动画，不引入另一套控件或绘图体系：

- **长单行输入**：原生 `TextInput`、`Autocomplete` 等输入未聚焦时，仅绘制布局按可用宽度显示省略号。完整编辑布局、实际值、鼠标拖选、光标导航、IME 与辅助技术不变；密码继续掩码。绘制布局只缓存在对应编辑器，不放入全局文字缓存。单行占位文字也以省略号结束。
- **复合输入占位文字**：固定版本的 `Placeholder` 只作用于控件自身，`SearchField`、`Combobox`、`TokenField` 外层未向内部输入转发，导致搜索范围文字不显示。现仅向 `field()` 明确标记的输入转发；完整编辑行为不变。单行占位文字沿用原生单行省略布局，而非 `NoWrap` 后直接裁切。
- **字幕邻近行平滑定位**：`ListState.ScrollToAnimated` 将已测量的可见行位置交给现有 `ScrollState.ScrollTo`，沿用原生缓动、用户滚动中断当前动画和系统减少动态效果设置；远距离或未测量目标仍由原生 `ScrollTo` 定位，不按估计高度长距离飞行。列表首尾导航、切换轨道和过滤会清掉旧动画；应用的后续播放跟随不被用户滚动暂停。

`tools/prepare_mygo.py` 校验九个上游文件 SHA-256，把锁定依赖复制到忽略目录（只复制内容，不清理无关文件；重新应用前移除补丁自身新增的测试文件）、依次应用两个补丁，再以生成的 `-modfile` 定向替换 MyGo。Go 不允许覆盖模块缓存的 overlay；三个统一构建/测试入口因此共用该生成模块文件，不修改模块缓存或已跟踪的 `go.mod/go.sum`，不跟踪完整上游源码副本。上游变化时哈希不符即停止，需重新评估补丁。

## 验收边界

- `TestOneSubInputElisionKeepsEditing`：长中英文地址、密码、失焦省略、聚焦尾部编辑与鼠标越界拖选，不缩短实际值。
- `TestOneSubCompositePlaceholderAndElision`：实际搜索、组合框、标签输入和建议输入的内部塑形布局，核对当前占位文字、单行省略、搜索清空后恢复，不以文案函数返回值代替显示验证。
- `TestOneSubRowScrollUsesNativeMotion`：可变行高的真实 Table，验证中间帧、最终位置、减少动态效果和首尾导航中断动画。应用回归另验证后续播放跟随不会暂停。
- 以上 UI 回归随 `build/test.ps1` 运行；不把组件测试当作全部桌面交互验收。

- `TestOneSubPopupItemWidth`：弹出真实菜单，用 `GetMenuItemRect` 读取菜单项宽度，要求不小于最宽文字所需宽度（旧算法实测 156 < 158 像素，失败；修复后通过）。
- `TestOneSubPopupFontBounds`：真实 GDI 字体，以 14/48/64 字号验证中文行高与留白。
- `TestOneSubPopupMonitorDPI`：96/144/192 DPI 下读取创建时的 DPI。
- 桌面实测：中文 Windows 11、96 DPI 单显示器下截取托盘菜单与预设菜单，文字墨迹与参考渲染逐列一致。多显示器混合 DPI 未实机验收。
