# SmartSub 样式适配与补丁

基线：`buxuku/SmartSub@f7ce2045aea53177a60236092487897da3c8cd06`，MIT。文件闭包见 [清单](../../manifests/smartsub-style.json)。

## 字体族别名优先级

文件：`main/helpers/fontResolver.ts`；符号：`registerFaces`、`discoverInstalledFonts`。

复现：Windows 安装 Microsoft YaHei / YaHei Light 或 Arial / Arial Black 时，后扫描的变体含同名 preferredFamily、subfamily=Regular，会覆盖精确字体族别名。选择 YaHei 最终解析成 YaHei Light，粗体变体缺失。

修改：别名按精确 family、fullName/PostScript、preferredFamily 排优先级，同级优先 Regular；清理目录索引时同时清理优先级。不修改 ASS builder、样式参数或预设，不重写字体解析体系。

适配器调用原 `loadFontData` 取得常规及粗体／斜体 variants，缓存经 SHA-256 校验的字节，并把同一组字体交给 JASSUB 和 FFmpeg。缓存位于用户数据目录，不随安装器再分发系统字体。

验证：真实 Windows 字体文件的 family 与 Bold 检查，重复缓存一致性；Go 包内样式与软／硬字幕合成回归；暂停帧粗体显示由桌面截图核对。

## 双语字幕事件

文件：`main/helpers/assStyleBuilder.ts`；符号：`buildAssDocument`。

需求：双语字幕的原文与译文各用一套样式，并在碰撞布局中保持为一组。

修改：可选参数 `bilingual` 增加 `Secondary` 样式行。每条字幕仍写成一个事件，用 `{\rDefault}` 和 `{\rSecondary}` 切换样式，`secondaryFirst` 决定上下顺序。块的对齐和边距沿用主样式。不传 `bilingual` 时输出与上游相同。

## 双语字体解析

文件：`main/helpers/assFonts.ts`；符号：`resolveAssFonts`。

根因：字体解析只以传入的主字体初始化事件，将任何 `\r` 都重置为主字体，没有读取 ASS 样式表。`Secondary` 字体虽然存在于样式行，却不进入提供给 JASSUB／FFmpeg 的字体集合；缺字处理也可能覆盖到错误字体。

修改：按 ASS `Format` 字段读取命名样式的字体；事件从自身 `Style` 的字体开始，裸 `\r` 回到该事件字体，`\rSecondary` 等命名重置使用对应样式。每次切换收集实际使用字体，不改写字号或其他样式字段。两层字体／字号、原译文对应与上下顺序分别测试，UI 明示主／次样式各自对应原文或译文。尚未配置的次样式从当前主样式继承字体，以 80% 字号初始化；用户保存的两套独立参数不被强制覆盖。

## 背景框精确边距

文件：`main/helpers/assStyleBuilder.ts`；符号：`FontMeasure`、`wrapForBox`、`exactBoxEvents`、`buildAssDocument`。

复现：背景框（BorderStyle=3）边距设为 0 时，左右贴紧文字，上下仍有空白。libass 按 OS/2 winAscent + winDescent 确定字号，背景框覆盖整个行高，与字形墨迹无关，`\ybord` 也不能小于 0。脚本空间为 384×288（4:3），16:9 视频中 libass 按画面宽度缩放水平边框、按高度缩放字形，所以左右边距大于上下边距。

修改：

- 传入 `measure` 时，不与其它字幕同时显示的字幕改用精确背景框。每个带框的行占一层：该层用不可见文字复用 libass 自身的背景框，再用 `\clip` 裁到"墨迹上下沿 ± 边距"的横带。相邻行的横带重叠时在中点分开，半透明背景不重复叠加。文字在最上层单独绘制。
- 自动换行改为显式 `\N` 加 `\q2`。换行按 libass 智能换行（WrapStyle 0）均衡行宽，并遵守中文避头尾。预览与烧录使用同一组行。
- `frameAspect` 校正水平边距：`\xbord = 边距 × (384 / 288) / 画面宽高比`。
- 与其它字幕时间重叠的字幕、字体无法测量的字幕和描边模式，保持上游输出。

适配器：`workers/style-core/main.ts` 的 `measureStyle` 用 fontkit 读取 SmartSub 交给 libass 的同一份字体字节（粗体取同族粗体字面），按 `Fontsize / (winAscent + winDescent)` 换算字宽与字形墨迹。

验证：`workers/style-core/box.test.ts` 固定事件结构。1920×1080 画面、Microsoft YaHei 24、FFmpeg 8.1（libass）烧录后逐像素测量：

| 边距 | 上 / 下 / 左 / 右（像素） | 上游输出 |
| --- | --- | --- |
| 0 | 1 / 1 / 3 / 3 | 21 / 14 / 7 / 7 |
| 4（=15 px） | 17 / 16 / 17 / 17 | 31 / 24 / 23 / 23 |
| 8（=30 px） | 31 / 30 / 33 / 33 | — |

4:3 画面边距 4 的结果同为 17 / 16 / 17 / 17。像素值含抗锯齿和字形左右间距。半透明多行背景框的行间可能有 1 px 浅缝。
