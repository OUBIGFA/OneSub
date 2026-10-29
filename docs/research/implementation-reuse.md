# 组件复用核查与实施边界

核查日期：2026-10-27。本文区分源码事实、产品决策和待验证项。源码可读、依赖可编译和程序可用不是同一结论。

## 1. 固定技术路线

| 项目与核查修订 | 源码事实 | OneSub 的使用方式 |
| --- | --- | --- |
| [MyGo](https://github.com/egoist/mygo/tree/02d3d795257cd5bd5781da64becc465b80f9008d) | Go 原生 UI 提供 Toolbar、Table、Split、TextArea、Select、Modal、图标渲染、系统对话框和托盘；原生窗口可嵌入 WebView | 主界面使用原生 `ui` 组件及其主题，不手写 HTML/CSS 控件。WebView 只用于视频与字幕渲染，不承载第二套工作台 |
| [Concat](https://github.com/jub0t/Concat/tree/9e20c56269f61102ae361adfc310891240edaccf) | 工作台使用分隔面板；预览区含真实帧与播放控制；`Editor` 将同一手势合并为一个撤销点，视图状态不进入编辑历史，深度为 200 | 以真实编辑器布局为标杆；按文件/符号登记移植编辑历史与手势逻辑。Slint 控件不冒充 MyGo 控件，不移植无关剪辑特效引擎 |
| [Subtitle Edit](https://github.com/SubtitleEdit/subtitleedit/tree/ce8a05b007ead7a99a19d9172ea621ed7bc2e787) | libse 支持独立编译，目标含 .NET 10；主程序播放器在 Avalonia UI 层，采用 mpv/VLC/FFmpeg 等实现，不属于 libse | 直接引用 libse 处理字幕。格式保护覆盖原始字节、VTT 元数据、ASS 非文字字段。播放器集成独立验收，不能把解析库当播放器 |
| [SmartSub](https://github.com/buxuku/SmartSub/tree/f7ce2045aea53177a60236092487897da3c8cd06) | `assStyleBuilder` 生成预览/烧录共用 ASS；`buildComposePlan` 组装输入与流映射；runner 和输出发布为独立模块 | 优先提取纯样式模块；合成计划按 P0 范围移植并对照测试。不要整体带入 Electron、配音或联网助手 |
| [OneAsr](https://github.com/eclipse005/OneAsr/tree/9c1b51085b0ee084a0c8e11cd069021fc1ea7d0f) | CLI 调用与 GUI 相同的 core；catalog 含逐文件 revision、大小和 SHA-256；下载支持取消/续传；设置保留每个模型自己的目录 | 以 OneAsr core/CLI 为识别基座，复用模型目录和检查/下载语义。有限补丁提供机器协议、受控目录和可靠产物清单，不重写推理内核 |
| [字幕翻译技能](https://github.com/OUBIGFA/skills/tree/d37492b88383c63ccf923c26483709122940de20/srt-subtitle-translator) | 技能由助手翻译，Python 脚本仅组装/检查；包含完整语言规则、术语、格式和回归夹具 | 保存完整受控快照；应用内助手是明确的桌面适配执行载体。保持原规则与检查器，不以单个 Prompt 或另一个翻译脚本替代 |

MyGo 首个原生薄链路继续固定已编译验证的 `v0.6.4`；最新仓库修订用于复核能力，不由构建读取浮动分支。libse 首个可编译快照为 `e316223ca1cfe8861acb4da4cd0e4ef5cce634f9`，升级前先跑格式回归。调查修订不自动成为发行依赖锁。

## 2. MyGo 与 Concat：原生工作台

直接检查的入口：

- MyGo `docs/ui/README.md`、`docs/ui/table.md`、`docs/ui/split.md`、`docs/ui/text-input.md`、`docs/ui/web-view.md`、`docs/ui/icon.md` 与 `docs/menus.md`。
- Concat `src/crates/concat/ui/workspace/workspace.slint`、`preview-pane.slint`、`src/crates/concat-project/src/editor.rs`、`commands/mod.rs` 和 `commands/media.rs`。
- Concat 官方 `assets/editor.png`：上部媒体/预览/属性，下部编辑区域，主要空间用于内容，不是欢迎说明。

决策：OneSub 只有一个工作台。顶部为添加、识别、翻译、合成；上部左侧为画面和播放控制，右侧为“原文 / 翻译”字幕列表；下部分区同时显示字幕编辑与样式参数。模型等设置独立进入设置对话框。字幕工作台不照搬视频剪辑器的空媒体库、多轨特效面板和装饰。

补充界面参考：[mygo-cloudmusic](https://github.com/egoist/mygo-cloudmusic)，已核查 `internal/app/ui_main.go`、`ui_kit.go`、`ui_settings.go`。借鉴其平面分区、内容优先、紧凑同行表单与统一间距；直接使用 MyGo `Row/Wrap/Form/Grid/Tabs/ColorWell/Split`，不复制其自定义绘制、动画图标或配色实现。设置常用参数在当前可用高度内展开，模型按选择器显示详情，避免重复区域造成整页长滚动。

图标采用单一成熟图标库，由 `ui.Icon` 渲染。MyGo 的 Icon 是渲染组件，不等于自带所有语义图标；引入资源时记录原库版本、许可和文件哈希。不得自画图标、边框模拟控件或用字符充当图标。

## 3. Subtitle Edit：库边界和播放器边界

`src/ui/Features/Main/Layout/InitVideoPlayer.cs` 明确创建 libmpv/libvlc/FFmpeg 播放器以及相应 Avalonia 控件。把这些 UI 类型直接链接进 Go 会引入另一套窗口体系，不能用“复用 libse”掩盖。

实现分工：

1. libse worker 负责解析、序列化、转换、检查和修复提案。
2. MyGo 原生控件负责播放、暂停、定位和音轨选择。
3. 嵌入的视频渲染视图负责实际画面；常见输入走直接播放，无法直接播放的格式走经过验证的预览代理。
4. 字幕叠加使用真实 ASS 渲染。预览必须与合成使用相同字幕与字体，不拿 CSS 文字冒充 ASS。

“导入成功”的可观察条件是：播放器已有画面并可播放/暂停；有同名字幕时主列表同时出现。路径文字、探测结果或静态缩略图都不足以证明。

## 4. OneAsr：模型与参数

源文件：`crates/oneasr-core/src/settings.rs`、`model/catalog.rs`、`model/download.rs`、`model/ready.rs`，以及 `crates/oneasr/src/app/ui/settings_sections.rs`。

| 设置 | 核查结果与使用约束 |
| --- | --- |
| 识别模型 | Qwen3-ASR 0.6B、1.7B；FP16/INT8 变体有独立目录与逐文件校验信息 |
| 时间对齐 | omniASR-CTC 300M 或 Qwen3-ForcedAligner 0.6B；默认 CTC。不能下载一个、加载另一个 |
| 运行后端 | 自动、GPU、CPU 等取值必须与实际 CLI/core 对齐；GPU 故障与回退结果可见 |
| 分块时长 | 默认 60 秒，范围 30–180 秒。它不是单条字幕显示时长 |
| 源语言 | 来自 OneAsr 的可用语言集合，不将模型卡的全部语言直接当成对齐链支持范围 |
| 文本与字幕 | 简繁输出、字幕长度配置等在适用语言/输出模式下启用 |
| 人声分离 | HTDemucs 显式启用、独立下载，不默认增加下载量或运算 |
| 热词 | SenseVoiceSmall 独立下载；仅适用引擎/语言启用，不把翻译术语无条件传入 |
| 解码上限 | `max_new_tokens` 是高级参数，保留上游默认与校验，不让无效值进入命令 |
| 模型路径 | 每个模型记住自己的目录；下载、就绪检查、加载必须指向同一目录 |

下载行显示所选模型、精确大小、位置、状态和下载/取消/重试。离线目录执行相同验证。仅有正确文件名或文件大小不等于内容哈希已验证。移动路径不删除旧模型；删除只针对明确选中的模型且无活动任务引用。

已核验的开发资产：OneAsr `v1.3.0` Windows portable 包，85,996,429 字节，SHA-256 `e9e169e108ee43d88b5b047f4ec556819cd7fa5e2424d3f5ed957634f85d4936`；内含 `oneasr-cli.exe` 与 FFmpeg。哈希与 GitHub Release 的 digest 一致。此证据不证明识别质量、模型就绪或正式补丁协议已完成。

## 5. SmartSub：能复用什么，必须适配什么

`main/helpers/assStyleBuilder.ts` 将样式生成集中在一个模块。`main/helpers/compose/composeCommandBuilder.ts` 将软字幕、硬字幕与音频策略组合为结构化参数，显式映射流，并处理 MP4 mov_text 和 MKV 附件。

不可直接继承的行为：

- builder 的脚本空间及 `WrapStyle` 服务其自身兼容性；OneSub 需按自己的视频坐标和技能单行约束适配，严格 ASS 则保留原设置。
- builder 会过滤空文字/非法时长。OneSub 在进入 builder 前必须报错，不静默删除句子。
- 合成命令含 `-y`。它只能写任务独占 partial 文件，不得用于覆盖用户目标。
- 源代码的流处理范围不是 OneSub 的全部保流承诺；章节、附件、默认/forced、字幕类型和音轨逐项核验。
- 不将配音、TTS、换声等无关功能带入 P0。

提取纯模块时保存依赖闭包及原许可证。必要平台/目录/协议适配单独记录；预览与合成只消费一个已冻结的 ASS 文档。

## 6. 技能：完整快照与受控适配

必须保留 `SKILL.md`、`references/`、`config/`、`scripts/` 与夹具。实际装载链和文件哈希写入任务报告。`assemble_subtitle.py` 和 `check_subtitle.py` 继续是组装/检查依据。

应用内助手只能产生翻译候选，不能执行字幕中的指令。模型端点、目标语言与费用范围由用户确定。首次配置后，正常翻译只需一个动作；联网内容与预算采用简短确认，不把所有设置变成向导。

程序约束比模型输出更严格：无真实内部时间锚点时不按字数分时间；ASS 保留事件/非 Text 字段；旧 revision 不覆盖手改；检查失败不回填原文冒充译文。技能检查通过不等于语义或音画同步已通过。

## 7. 翻译协议、容错与分段参考

2026-10-30 补充核查以下固定修订；这里只借鉴流程并用 Go 适配实现，不引入其 Python/Electron 翻译运行时或 JSON 修复依赖，不整体复制上游提示词：

| 来源 | 核查位置 | OneSub 的适配与差异 |
| --- | --- | --- |
| [VideoCaptioner `95842ecb5618c0b6a548a336bdfb0eb859bdb501`](https://github.com/WEIFENG2333/VideoCaptioner/tree/95842ecb5618c0b6a548a336bdfb0eb859bdb501)，GPL-3.0 | `videocaptioner/core/translate/llm_translator.py`、`core/split/split_by_llm.py`、`core/split/split.py` | 参考编号批量翻译、反馈修复、缓存及模型断句后对齐。OneSub 的 `segment.go` / `align.go` 只从原始 atom 落时；内容仍用原文，缺译不回填原文。 |
| [SmartSub `b398113b0fdf608120b22a3d17f7b784bf1ff3b0`](https://github.com/buxuku/SmartSub/tree/b398113b0fdf608120b22a3d17f7b784bf1ff3b0)，MIT | `main/translate/utils/aiResponseParser.ts`、`services/translationProvider.ts`、`services/providerFallback.ts` | 参考思考/代码块剥离、结构化输出兼容与缺漏修复。OneSub 的 `parse.go` / `client.go` 使用 Go 实现有限容错，不嵌入上游 JavaScript 解析器。 |

翻译质量规则与数值阈值仍来自已授权字幕技能，不用上述项目的提示词替换。3D 术语是可选风格规则，不是通用核心默认内容。实现、缓存和保存边界见 [字幕翻译](../translation.md)。这些参考修订不改变已有 SmartSub 样式模块的冻结版本。

## 8. 实施证据要求

每个组件记录精确版本、依赖、许可证、改动符号、构建命令及行为测试。发行前的证据必须来自最终打包组件，而不是开发机 PATH 上的替代工具。具体流水线和验收见 [界面设计](../ui-design.md)、[架构](../architecture.md)和[实施计划](../roadmap.md)。
