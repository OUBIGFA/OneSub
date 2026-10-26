# 成熟字幕方案调研与设计依据

调研日期：2026-10-26。本次核查官方源码、许可证、模型卡和商业产品文档；未执行第三方应用的安装、付费试用、识别基准或 GPU 测试。下文“官方说明”只证明厂商公开描述了该能力，不证明质量、性能或当前套餐可用性。

机器可读的 [证据索引](evidence.json)记录 URL、捕获状态、响应哈希和阅读用途。源码链接固定 commit；商业网页会变化，哈希用于区分捕获内容，不保证未来仍可下载同一页面。

## 1. 结论

OneSub 的可行路线是复用成熟组件，再把工程数据、人工校对和交付验证统一起来。最需要先解决的是多运行时打包、时间证据、严格格式保护和技能授权。界面应减少入口与术语，将“处理什么、发给谁、会改哪些内容、如何恢复”放在操作发生的位置。

功能收益主要来自四类成熟实践：字幕编辑器的精确校时与格式保护、AI 字幕工具的批次审阅与术语预审、商业工具的清晰导出与样式入口，以及本地识别工具的隐私和模型管理。产品不以模型数量或底层库支持的格式总数作为已实现能力。

## 2. 开源组件与实际复用边界

| 项目与固定证据 | 已核查事实 | OneSub 决策与验证要求 |
| --- | --- | --- |
| [MyGo](https://github.com/egoist/mygo/tree/558e498142b0e91b8e659f7e1f9aae5a455108f3) | go.mod 要求 Go 1.27.3；前端文档说明 Web 模板与资源机制；MIT 文件可读 | 采用 Go/Web 桌面壳；G0 必须实际构建并验证 WebView 媒体，不能把源码可见等同跨平台可用 |
| [Subtitle Edit/libse](https://github.com/SubtitleEdit/subtitleedit/blob/e316223ca1cfe8861acb4da4cd0e4ef5cce634f9/src/libse/LibSE.csproj) | libse 目标 netstandard2.1 与 net10.0，依赖 SkiaSharp、UTF.Unknown；libse 的 MIT 文件可读 | 直接编译自包含 .NET worker；串行管理全局配置；格式与 native assets 单独验收 |
| [OneAsr CLI](https://github.com/eclipse005/OneAsr/blob/9c1b51085b0ee084a0c8e11cd069021fc1ea7d0f/crates/oneasr-core/src/bin/oneasr-cli.rs) | transcribe、align、render 各有参数；timeline、VAD、model catalog 与语言定义可读 | 保留内核，只做机器协议和任务目录等有限补丁；按真实子命令适配，不解析人类日志作为协议 |
| [Concat 编辑器](https://github.com/jub0t/Concat/blob/94f4255f1b574485262b86b2e9f0b341bc9bf731/src/crates/concat-project/src/editor.rs) | 编辑历史与手势逻辑可读；Cargo 声明 AGPL-3.0-or-later | 借鉴并按许可移植项目自有编辑逻辑；React 实现对应交互，不复制 Slint/vendor 或完整视频剪辑引擎 |
| [SmartSub ComposePlan](https://github.com/buxuku/SmartSub/blob/f7ce2045aea53177a60236092487897da3c8cd06/main/helpers/compose/composeCommandBuilder.ts) | 类型、容器与合成决策可读；MIT 文件可读 | 登记后抽取纯 TS 样式部分，Go 负责进程与文件；样式参数最终冻结为一份 ASS，不引入 Electron 常驻后端 |
| [字幕翻译技能](https://github.com/OUBIGFA/skills/blob/d37492b88383c63ccf923c26483709122940de20/srt-subtitle-translator/SKILL.md) | 规则与检查器要求保真、来源覆盖、时间约束及不同字幕模式 | 完整受控快照加桌面执行适配；权利人已授权自有内容，第三方材料仍待引入时核查；不把脚本当翻译 SDK，不以其他项目提示词替换 |
| [JASSUB 发布清单](https://github.com/ThaUnknown/jassub/blob/482a2fed6c8976550f09383e90d92b6ac3360fc9/package.json) | 版本 2.5.23；发布包包含 WASM/Worker；license 字段是多个许可证组合 | 交互 ASS 预览候选；共享 ASS/字体并做实帧比对；能力不足改精确静帧/样片，不能以 CSS 假装一致 |
| [VideoCaptioner 翻译器](https://github.com/WEIFENG2333/VideoCaptioner/blob/95842ecb5618c0b6a548a336bdfb0eb859bdb501/videocaptioner/core/translate/llm_translator.py) | 批次翻译、结果映射、校验和异常处理源码可读 | 借鉴结构校验与有限修复；缺失译文不得回填原文冒充完成，不复制其翻译内核 |
| [VideoLingo 摘要](https://github.com/Huanshere/VideoLingo/blob/01f843492759be9bbfd1a5cb697b799cc9de1609/core/_4_1_summarize.py) | 全局主题/术语流程可读，摘要输入存在截取边界 | 采用术语预审；长片分片阅读再汇总，不把开头摘要标为读过全片 |
| [pyVideoTrans](https://github.com/jianchang512/pyvideotrans/blob/a52eeee4fd97ea781de6b986c065dabe86d621d5/README.md) | README 说明识别、翻译、字幕、配音等多阶段工作流 | 纳入队列、人工校对与恢复的产品考虑；TTS/声音克隆不进入 P0 |
| [Aegisub 文档](https://aegisub.org/docs/latest/overview/) | 字幕网格、音视频、时轴和样式工具的官方概览 | 核心编辑对象保持单一；精确时间编辑有键盘替代；ASS 有意分层与重叠不能按普通对白去重 |
| [whisper.cpp](https://github.com/ggml-org/whisper.cpp/tree/231638a0717b7822784297e5980965ffcec05810)、[faster-whisper](https://github.com/SYSTRAN/faster-whisper/tree/9e472ae9de8bb374552f531f752654554b76b2d6) | README 说明本地推理、后端和批处理能力 | 用同一语料做质量/资源对照；不把第二套识别运行时放进默认安装包 |
| [Wails v2.10.2](https://github.com/wailsapp/wails/tree/e753157ad2a2840896dd887ad7d4263747774805) | Go/Web 桌面路线和平台说明可读 | 用作框架比较；MyGo 通过门禁前不宣称同等能力，也不无依据启动框架迁移 |

许可表仅对已经核查的具体文件作结论。没有在本次复查中完整审计全部传递依赖；未来真正复制或发行时以实际版本的许可材料为准。

## 3. 商业产品工作流

| 产品与一手来源 | 官方公开说明 | 借鉴与边界 |
| --- | --- | --- |
| [Descript Captions](https://help.descript.com/visuals/captions.md)、[字幕导出](https://help.descript.com/export-and-share/subtitles.md) | 从 script 建立字幕层；样式预设可搜索/过滤；场景或选中层有明确作用域；SRT/VTT 作为独立文件导出 | 将翻译风格与字体外观分开；显示全局/局部作用域；用“字幕文件/固定字幕视频”解释交付。OneSub 不把改文字隐式变成剪辑媒体 |
| [MacWhisper](https://www.macwhisper.com/) | 拖放转写，本地模型与离线私人文件处理 | 首次入口简短；没有云账号可工作；应用、模型和缓存体积分开显示。没有实测其语言质量或处理速度 |
| [VEED Subtitle Editor](https://www.veed.io/tools/subtitle-editor) | 在线修改、同步、样式和字幕生成工作流 | 让普通用户先选任务与结果，再展开参数；不照搬其云上传作为默认，不采纳营销准确率 |
| [DaVinci Resolve Edit](https://www.blackmagicdesign.com/products/davinciresolve/edit) | 字幕时间线、多语言轨、轨级样式，以及独立文件/视频内交付 | 多语轨与样式作用域进入 P1；容器能力与样式损失先预检；不复制完整视频剪辑工作区 |
| [SubtitleNEXT](https://subtitlenext.com/) | 字幕创建、校正、样式、QC、批量及专业本地化工作流 | 质量问题可定位、批次审阅状态可追溯；广播规范和格式按版本验收，不因为专业产品支持就宣称 OneSub 支持 |

上述商业产品未在本次运行验收。价格、额度、准确率、实际套餐权限与商用许可不作比较结论，采购前必须另行核对。

## 4. 关键技术证据与风险

### Subtitle Edit：双向自动关联

在同一固定快照中核查到以下实际调用，未运行其桌面程序：

- [MainViewModel.cs:26674](https://github.com/SubtitleEdit/subtitleedit/blob/e316223ca1cfe8861acb4da4cd0e4ef5cce634f9/src/ui/Features/Main/MainViewModel.cs#L26674)：字幕打开后，在 AutoOpen 开启时依次考虑显式视频、最近关联和文件名发现。
- [MainViewModel.cs:37924](https://github.com/SubtitleEdit/subtitleedit/blob/e316223ca1cfe8861acb4da4cd0e4ef5cce634f9/src/ui/Features/Main/MainViewModel.cs#L37924)：视频区接收拖放，字幕走 SubtitleOpen，视频打开后调用 TryLoadAssociatedSubtitle；后者处理已加载与未保存内容，并用 skipLoadVideo 避免重复打开。
- [FindVideoFileName.cs](https://github.com/SubtitleEdit/subtitleedit/blob/e316223ca1cfe8861acb4da4cd0e4ef5cce634f9/src/ui/Logic/Media/FindVideoFileName.cs)、[FindSubtitleFileName.cs](https://github.com/SubtitleEdit/subtitleedit/blob/e316223ca1cfe8861acb4da4cd0e4ef5cce634f9/src/ui/Logic/Media/FindSubtitleFileName.cs)：按扩展名查找，并包含点号/下划线截短、语言后缀等回退；部分路径按首个候选返回，视频查找还受父目录搜索设置影响。

OneSub 将双向同名自动配对设为默认开启的 P0 首要入口，并让添加与拖放行为一致。唯一同目录同名文件直接加载；多个候选显式选择。P0 不采用跨目录和任意截短名称的自动回退，以免把不同章节或版本配错；带语言后缀的非精确命中只给候选。该差异是产品决策，不宣称复刻 Subtitle Edit 全部匹配规则。

### 其他边界

- **识别语言有边界。** [OneAsr lang.rs](https://github.com/eclipse005/OneAsr/blob/9c1b51085b0ee084a0c8e11cd069021fc1ea7d0f/crates/oneasr-core/src/lang.rs)列出 11 种源语言，热词只适用中文与英文。模型卡的语言数量不能直接变为应用支持列表。
- **时间来自输入证据。** [timeline.rs](https://github.com/eclipse005/OneAsr/blob/9c1b51085b0ee084a0c8e11cd069021fc1ea7d0f/crates/oneasr-core/src/timeline.rs)的词时间单位为秒，不提供 OneSub cue ID 或逐词 confidence。适配器需要显式音轨、零点和不连续时间映射。
- **权重许可独立核查。** [Qwen3-ASR](https://huggingface.co/Qwen/Qwen3-ASR-0.6B/blob/main/README.md)、[omniASR](https://huggingface.co/facebook/omniASR-CTC-300M/blob/main/README.md)、[SenseVoiceSmall](https://huggingface.co/FunAudioLLM/SenseVoiceSmall/blob/main/README.md)是模型资料入口；发行时还需冻结转换来源、镜像与权重哈希。模型下载量不等于运行内存。
- **格式丰富不代表无损。** libse 的重序列化能力不能替代严格 ASS 的非 Text 字段保护；VTT settings、NOTE/STYLE/REGION 与多人重叠需要专门夹具。
- **预览不是最终产物证明。** JASSUB 与 FFmpeg/libass 共享输入仍需字体、换行、坐标、时轴的实帧检查；跨平台不承诺像素级零差异。
- **数据库备份须一致。** [SQLite WAL 文档](https://sqlite.org/wal.html)说明 WAL 运行与文件系统约束；活动工程使用本地可靠文件系统和一致性备份，不直接同步活动数据库目录。
- **分发依构建内容判断。** [FFmpeg Legal](https://ffmpeg.org/legal.html)明确构建选项影响许可；独立进程不自动免除义务。当前不随仓库分发这些二进制。
- **易用性必须可测。** [WCAG 2.2](https://www.w3.org/TR/WCAG22/)提供重排、对比度、目标大小和输入替代的依据；OneSub 具体用户任务与读屏验收见[界面设计](../ui-design.md)，尚未声明全面符合。

## 5. 功能落实

| 用户问题 | 设计落点 | 阶段 |
| --- | --- | --- |
| 添加视频还得再找字幕，添加字幕还得再找视频 | 拖放/添加统一，双向同目录同名自动配对；多候选可选 | **P0 首要** |
| 识别错音轨、无谓重复识别 | 音轨试听，已有文本字幕优先提取 | P0 |
| 手改被 AI 覆盖、失败后无法恢复 | 人工锁、revision 检查、来源快照、事务与恢复 | P0 |
| 不清楚发往云端的内容与费用 | 发送范围、端点、预算与结果未知状态 | P0 |
| 文字正确但时轴/输出不一致 | 真实锚点、冻结 ASS、5 秒样片、最终产物检查 | P0 |
| 术语不一致、系列内容重复劳动 | 最小术语预审；完整术语库与翻译记忆后续扩展 | P0→P1 |
| 多语审校难回导 | 稳定 ID、基准版本、匹配与冲突预览 | P1 |
| 专业工具功能过多，初学者找不到入口 | 三个任务入口，准备/编辑/导出，一处主列表 | P0 |

## 6. 未验证项与证据质量

EZTitles 的目标页面、Adobe Speech to Text 页面及 FFmpeg Security 页面在捕获时不可用，因此未据此作能力或安全状态结论。Descript 的旧帮助链接重定向到首页，不能证明字幕细节；本次另读取正确的 Captions 与 Export subtitles 官方文档。

所有组件的构建、GPU 路径、安装体积、识别/翻译质量和真实用户易用性均待[实施验收](../roadmap.md)。研究 SHA 定义阅读快照，不代表已完成发行依赖冻结。本地抓取全文、私人输入和会话文件不进入公开仓库。
