# MyGo 原生 UI 与 Windows 层补丁

上游：`egoist/mygo v0.8.3`，修订 `5add3817c74bb7505d0b473bc5fb28abe2201b99`，MIT，许可见 [mygo.txt](../../licenses/mygo.txt)，模块、源码 ZIP 与许可校验见 [运行时清单](../../manifests/runtimes.json)。不替换菜单或绘制应用自有控件；保留 MyGo / Windows 的菜单渲染和行为。

## 范围与原因

`windows-native.patch` 修改 `internal/windows/menu.go`、`menudraw.go`、`surface.go`、`webview_embed.go`，并在上游测试文件中增加回归。菜单部分的范围是 MyGo 自绘的全部弹出菜单项：托盘菜单、按钮菜单（如预设的“更多”菜单）、右键菜单和文本编辑菜单。

- **弹出菜单宽度**：上游 `v0.8.3` 的 `itemWidth`／`learnMenuExtra` 按实际绘制宽度学习 Windows 给自绘菜单附加的空间；首个菜单完整预留，后续按实测值扣除，不再使用 `SM_CXMENUCHECK` 估算。本轮移除 OneSub 的 `ownerDrawExtra`、字体平均字宽算法与缓存字段，直接使用上游实现。原生回归按应用流程重建同项菜单两次，要求两次均不窄于完整文案，第二次使用已学习宽度。
- **字形外伸被裁切**：0.8.3 已在标签和快捷键绘制中使用 `DT_NOCLIP`，并让快捷键与标签共用基线。本次移除 OneSub 的重复常量与绘制标志补丁，保留上游的中文菜单像素回归。
- **显示器 DPI**：菜单建在指针所在显示器，托盘 owner 是隐藏窗口，DPI 不代表菜单所在显示器。现在把创建时的 DPI 保存在菜单项中，度量与绘制读取同一值。
- **调整区域时视频区露白（实测根因）**：原生 UI 中的网页视图（播放器）经 DirectComposition 显示在画布子窗口之上，其下是该子窗口自己的位图；MyGo 对该窗口的擦除背景消息不做绘制，位图保持创建时的白色。快速拖动分隔条使播放器区域变大、网页尚未按新尺寸重绘时，这块白色露出。现在有网页视图时，擦除背景用窗口的背景色（OneSub 设为黑色）填充，并在创建网页图层和窗口尺寸变化时触发擦除；不额外叠加色块。
- **快速拖动时旧工具栏进入播放区（实测根因）**：原生 UI 的首帧先创建 HWND flip 交换链，WebView 图层出现后又切换到 DirectComposition。旧交换链释放后，窗口的重定向表面仍可能保留最后一帧；透明媒体区域移动时暴露这组旧像素。现在从首帧就使用同一组合呈现路径，不关闭 GPU、局部重绘或暂停播放器。仅清除位图、完整重绘与延后定位的对照改动未解决此问题，均不保留。此原生路径与 Chromium 的视频呈现兼容参数相互独立。
- **放大文字**：原菜单行固定 32 DIP，字体随系统辅助功能文字大小放大。现在取固定行高与“文字行高＋内边距”的较大值。

`ui-interaction.patch` 复用固定版本已有的单行文字塑形和滚动动画，不引入另一套控件或绘图体系：

- **长单行输入**：原生 `TextInput`、`Autocomplete` 等输入未聚焦时，仅绘制布局按可用宽度显示省略号。完整编辑布局、实际值、鼠标拖选、光标导航、IME 与辅助技术不变；密码继续掩码。绘制布局只缓存在对应编辑器，不放入全局文字缓存。单行占位文字也以省略号结束。
- **复合输入占位文字**：`v0.8.3` 的 `Placeholder` 已通过原生 `focusTo` 向 `SearchField`、`Combobox`、`TokenField` 内部输入转发。本轮移除 OneSub 的递归查找转发补丁，直接使用上游接口；保留中英文当前范围及清空搜索的行为回归。单行长占位文字仍通过有限布局补丁使用省略号，而非 `NoWrap` 后直接裁切。
- **字幕邻近行平滑定位**：`ListState.ScrollToAnimated` 将已测量的可见行位置交给现有 `ScrollState.ScrollTo`，沿用原生缓动、用户滚动中断当前动画和系统减少动态效果设置；远距离或未测量目标仍由原生 `ScrollTo` 定位，不按估计高度长距离飞行。切换轨道和过滤会清掉旧动画；应用的后续播放跟随仍自动恢复。
- **字幕首尾导航的缓出反馈**：官方 [List](https://mygo.egoist.dev/docs/ui/list) 的首尾定位没有动画参数；[Motion](https://mygo.egoist.dev/docs/ui/motion) 和 [Scroll view](https://mygo.egoist.dev/docs/ui/scroll) 已提供 `CubicBezier` 与 `ScrollState.ScrollTo`。有限扩展 `ListState.ScrollToEdgeAnimated` 先使用原生布局测量目标与最多一屏的落点走廊，再将这段滚动交给既有 `ScrollState`；远段直接跳过，不遍历整个列表的估计高度。应用采用 420 ms、`CubicBezier(0.22, 1, 0.36, 1)`，先快后慢、无回弹；末条超过一屏时仍定位到真实底部。用户滚动、反向导航和直接定位可中断，减少动态效果时直接到达。不修改普通播放跟随的 220 ms 节奏。

`ui-layout.patch` 处理固定版本 WebView 的原生尺寸缺口，并为应用首帧验收提供单帧测试入口；不引入独立渲染器或控件体系。官方接口核对见 [Web view](https://mygo.egoist.dev/docs/ui/web-view) 与 [Layout](https://mygo.egoist.dev/docs/ui/layout)。

- **媒体本帧定位**：`WebView` 的 `Fit(Contain)` 配合 `AspectRatio`，在最终内容框内居中计算媒体矩形；原生视口和透明区域使用同一矩形。应用不再先读上一帧 `Bounds()` 后设置视频尺寸。未使用此组合的 WebView 行为不变。
- `TestOneSubContainedWebViewUsesCurrentFrame` 每次只执行单帧尺寸变化，验证横竖比例、原生位置与透明区域立即一致。既有上游 `TestWebView*` 同时运行。
- **真实单帧验收**：上游 `Tester.Frame()` 会自动运行最多 20 帧，不能证明第一帧已稳定。有限扩展 `Tester.FrameOnce()` 只执行一帧，不排空后续无效化；既有 `Frame()` 行为不变。应用用刻意延后一帧的宽度变化验证该入口确实能暴露跳动，再逐页比较第一帧与稳定帧，覆盖智能路由、指定模型、队列、中英文、浅深色和 100%/150%/200% 文字缩放。
- 这些回归只证明布局与输入契约；桌面残影需另行复现和像素验证，不把结构修复等同于全部 GPU 问题已解决。

`ui-row-click.patch` 扩展固定版本的 `ListState` 原生行事件，修改 `ui/list.go` 与 `ui/value_input.go`，不绘制或替换列表：

- `OnRowClick` 包括重复点击已选行；双击在第二次按下时单独回调，不在第二次释放后再次执行单击。未设置回调的列表保持既有行为。
- `KeepClickPosition` 只禁止鼠标选择附带的自动滚动，原生选择、焦点、右键和键盘导航仍保留。应用立即选中字幕，将播放动作延迟到 Windows [GetDoubleClickTime](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-getdoubleclicktime) 判定结束；双击取消待执行的单击，期间保持目标行不移动。
- 应用回归覆盖播放/暂停下的 18 组单/双击动作组合，以及跟随开启/关闭下的 12 组单击动作、重复已选行、选择保留与定位恢复。它们使用真实 MyGo 输入和工程事务；最终桌面还须核对真实播放器与原生弹出菜单。

`ui-sidebar.patch` 修改 `ui/sidebar.go` 的选中项样式，不替换或绘制侧栏：

- 上游 `v0.8.3` 的 `Sidebar` 仅在自身持有键盘焦点时用强调色显示选中项，失去焦点改为灰色（同 AppKit）。设置对话框打开后焦点在下一帧才落到侧栏，点击内容区空白处又失去焦点，选中项因此在蓝、灰之间跳变。补丁让选中项始终使用强调色；键盘焦点继续由上游已有的焦点环表示，方向键、点击、辅助技术角色不变。
- `TestOneSubSidebarChosenKeepsAccent` 比较侧栏持有与失去焦点时选中项的像素，去掉补丁时失败。应用侧 `TestPagesAreSettledInTheirFirstFrame` 逐页比对设置首帧与稳定帧；`TestSettingsSidebarKeepsAccentAcrossBlankClicks` 在真实设置视图中点击标题空白，并切换窗口焦点，逐帧核对所有分类的选中项像素。

`tools/prepare_mygo.py` 校验十三个上游文件 SHA-256，把锁定依赖复制到按版本隔离的忽略目录 `.local/mygo-menu-source-v0.8.3/`（只复制内容，不清理无关文件；重新应用前移除补丁自身新增的测试文件）、依次应用五个补丁，再以生成的 `-modfile` 定向替换 MyGo。镜像指纹同时包含锁定版本与补丁内容，升级时即使补丁未变也会重建源码镜像。Go 不允许覆盖模块缓存的 overlay；三个统一构建/测试入口因此共用该生成模块文件，不修改模块缓存或已跟踪的 `go.mod/go.sum`，不跟踪完整上游源码副本。上游变化时哈希不符即停止，需重新评估补丁。

版本锁定用于复现当前构建，不是长期停留在旧版的理由。按项目本地工作约定持续同步上游最新版本；每次升级重新核对源码校验值、API 与这组补丁，已有上游能力不再重复保留补丁。完整构建、回归和相关桌面验收未通过时记录实际阻塞，不宣称升级完成。

## 验收边界

- `TestOneSubInputElisionKeepsEditing`：长中英文地址、密码、失焦省略、聚焦尾部编辑与鼠标越界拖选，不缩短实际值。
- `TestOneSubCompositePlaceholderAndElision`：实际搜索、组合框、标签输入和建议输入的内部塑形布局，核对当前占位文字、单行省略、搜索清空后恢复，不以文案函数返回值代替显示验证。
- `TestOneSubRowScrollUsesNativeMotion`：可变行高的真实 Table，验证中间帧、最终位置、减少动态效果和直接定位中断动画。应用回归另验证后续播放跟随自动恢复。
- `TestOneSubEdgeScrollUsesNativeLanding`：20,000 行真实虚拟 Table，核对首尾定位的每四分之一段位移逐次缩小、最多一屏的落点、超高末行、选择不变与减少动态效果。
- `TestOneSubEdgeScrollCanBeInterrupted`：核对滚轮、反向首尾导航与直接定位中断旧动画。应用回归通过实际导航按钮验证缓慢收尾，并覆盖原始／优化字幕、筛选、实时译文与中英文／缩放布局。
- 以上 UI 回归随 `build/test.ps1` 运行；不把组件测试当作全部桌面交互验收。

- `TestOneSubNativeSurfaceStartsComposed`：建立实际原生窗口，在任何 WebView 尚未存在时检查首帧的组合交换链契约；随菜单测试一起运行。它不替代真实桌面的快速分隔拖动与视频像素验收。
- `TestOneSubPopupItemWidth`：弹出真实菜单两次，用 `GetMenuItemRect` 读取菜单项宽度；首个完整预留，后续复用上游学习的附加宽度，均要求不小于最宽文字所需宽度，第二次不超过必要宽度加 2 DIP。
- `TestOneSubPopupFontBounds`：真实 GDI 字体，以 14/48/64 字号验证中文行高与留白。
- `TestOneSubPopupMonitorDPI`：96/144/192 DPI 下读取创建时的 DPI。
- 桌面实测：中文 Windows 11、96 DPI 单显示器下截取托盘菜单与预设菜单，文字墨迹与参考渲染逐列一致。多显示器混合 DPI 未实机验收。

未选中的设置分类图标使用主题 `TextMuted`，强调色只标记当前分类；继续使用原生 `SidebarItem` 的布局、语义、焦点与鼠标反馈。

## 0.8.3 升级复核

本次保留菜单 DPI、放大字行高、组合呈现路径、长输入省略、列表点击和滚动、首帧检查及侧栏选择补丁。WebView 补丁只将本帧内容矩形用于包含式布局；上游新增的缩放、圆角、原生定位和未处理按键传递路径原样保留。五个补丁均重新基于 0.8.3 生成；不复制 0.7.4 的整个控件实现。

0.8.3 的 `Field` 在内部控件自带名称时为外层生成同名组。测试驱动在模态窗口中将该组的祖先误判为背景，拒绝本应到达输入框的点击。`ui-layout.patch` 仅在命中链包含该组、实际目标确为该字段控件或其委托输入时接受点击；遮挡检查仍保留。`TestOneSubNamedModalFieldTargetsItsControl` 验证输入生效、禁用后不写入和对话框不被误关闭。该修正只影响测试驱动。
