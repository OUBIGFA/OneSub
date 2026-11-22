# 第三方来源与许可规则

工作台组件与主题使用 MIT 的 [Lobe MyGo](patches/lobe-mygo/README.md)，底层窗口、布局、虚拟表格与媒体承载使用 MyGo。旧版 WinOne 和 Fluent UI 设计来源记录保留用于追溯，不再作为当前界面的主题实现。

当前已引入 MyGo、官方 MCP Go SDK、libse worker、SmartSub 样式模块、OneAsr 模型目录与开发运行时、完整字幕技能快照、Lucide 图标以及 JASSUB 播放渲染依赖。所有识别模型及自动语言检测共用 FireRedVAD `v1.0.0` 的 VAD 候选与 AED 说话确认，使用同一已打包二进制和默认模型参数，见 [集成登记](ports/oneasr/README.md)。SenseVoice、Paraformer 与 Parakeet-TDT 由 Apache-2.0 的 sherpa-onnx `v2.0.1` 官方 CPU 运行时执行，模型由用户按固定修订下载，不随安装器分发，见 [集成登记](ports/sherpa-onnx/README.md)。精确记录位于 `manifests/`，移植边界位于 `ports/`。

发行需保留传递许可、对应源码、构建材料、SBOM、摘要及真实签名状态。Windows 桌面、Windows 10、干净机器、安装、升级、失败回滚和卸载验收不作为构建或发布门槛，未实测的范围如实标注。JASSUB 的包装层 MIT 不代表其 WASM 只有 MIT 许可。

对应源码与构建材料登记在 [材料清单](manifests/distribution-materials.json)，原始许可来源与摘要登记在 [通知来源](manifests/material-notices.json)。发行随包保留通知正文；源码材料留在本地，并在公开二进制前通过经确认的合规渠道提供，不自动作为 Release 附件上传。恢复固定输入和重建说明见 [分发源码说明](../build/DISTRIBUTION-SOURCES.md)。源码集合含保守的开发／非 Windows 依赖超集，不冒充完整静态链接符号清单。

播放器许可登记见 [player-licenses.json](manifests/player-licenses.json)。abslink、lfa-ponyfill 和 throughput 的发布包明确声明 Apache-2.0 或 MIT；随包附标准许可正文、原有作者署名和来源校验，不编造版权年份。throughput 保留其来源 speedometer 的原许可。JASSUB 固定源码的库许可正文见 [jassub-bundled-libraries.txt](licenses/jassub-bundled-libraries.txt)，完整对应源码与安装包内许可材料仍单独核对。

TXT 文稿导出直接调用固定 libse 的 `HtmlUtil.RemoveHtmlTags`、`HtmlUtil.RemoveOpenCloseTags`、`Utilities.RemoveSsaTags` 与 `AdvancedSubStationAlpha.RemoveDrawingTag`；没有复制上游导出界面或引入新的运行时。字幕 worker 的定时写入与格式保护保持不变。

文件名分级关联直接调用 MIT 的 `agnivade/levenshtein v1.2.1` 计算 Unicode 编辑距离，规范化与语言代码解析复用既有 `golang.org/x/text`。精确修订、模块／下载校验、许可与测试专用依赖见 [文件名匹配清单](manifests/filename-matching.json)。不复制上游算法，不采用 Subtitle Edit 按首个文件返回或任意截短名称的匹配实现。

字幕阅读负荷与智能拆分的算法边界见 `ports/subtitle-splitting/README.md`。没有复制 Subtitle Edit、stable-ts 或 VideoCaptioner 的拆分代码。中文直接筛选 MIT 的 jieba 固定词频／词类词典并编译为嵌入式双数组 Trie，结合 Apache-2.0 BudouX 0.10.3 简繁中文模型的边界评分；日文／泰文保留既有 BudouX 模型。英文直接引用 MIT 的 prose 3.2.3 分词与词性模型，上游明确授权模型按 MIT 分发。见[集成登记](ports/subtitle-splitting/README.md)、[调研记录](ports/subtitle-splitting/RESEARCH.md)及[固定清单](manifests/subtitle-splitting.json)。没有新增 CGO、Python 或网络运行时；中文字典和模型以可复现 UTF-8 文本嵌入，英文模型通过 Go 模块嵌入，不扩大仓库二进制例外。Unicode 字素／词界直接调用现有 `Unlicense OR BSD-3-Clause` 的 `go-text/typesetting v0.3.5`，字符宽度使用既有 `golang.org/x/text v0.43.0`。字幕技能的阅读负荷阈值由轻量本地检测使用，许可沿用完整技能快照登记。

实际翻译协议与分段流程参考的固定上游修订、源码位置和适配边界见各组件的 `ports/` 登记。它们不是新增运行时依赖；完整字幕技能快照仍为质量规则来源。

翻译模型连接测试参考 [Cherry Studio 的模型检查](https://github.com/CherryHQ/cherry-studio/blob/563c1e8a8e4a1f5fc6f51fa4abbad5f9399e51d3/src/main/ai/AiService.ts)与[逐模型结果处理](https://github.com/CherryHQ/cherry-studio/blob/563c1e8a8e4a1f5fc6f51fa4abbad5f9399e51d3/src/renderer/pages/settings/ProviderSettings/ModelList/checkModelsHealth.ts)。实现直接复用 OneSub 既有 `translator.service.send`、Chat Completions／Responses 解析与凭据读取，不复制 Cherry Studio 代码或增加运行时依赖。测试只发送固定短句，等待完整非空文本，总时限 15 秒，不重试或重定向，不创建字幕或对话历史；结果与响应时间显示在原生 Lobe MyGo 模型表格中。

旧版浅色主题的 WinOne 来源、许可副本、哈希与适配边界见 [历史配色登记](ports/winone/README.md)；当前浅深色主题均使用 Lobe MyGo 的主题令牌。

MyGo 原生 UI 锁定为 2026-11-21 核查时的最新正式发布 `v0.10.0`，满足当前 Lobe MyGo `v0.0.0-20261116124832-d88b4ad7f238` 的依赖；源码 ZIP 与许可校验见 [运行时清单](manifests/runtimes.json)，有限补丁与行为边界见 [补丁登记](patches/mygo/README.md)。

MyGo 的应用更新直接复用 `mygo.Updater.Check`／`Update.Install`，包括 HTTPS／系统代理、Ed25519 校验、解包与替换／回滚；没有引入另一套下载器或安装器。设置“关于”与任务协调由应用原生 UI 承担，产品版本为 `0.1.2`，用数值第四段区分内部构建。归档与签名由 MyGo 构建器生成，只调整清单中的内部构建号与不可变 Release URL，见 [构建与更新说明](../build/README.md)。新增 GitHub 和“关于”图标继续直接复用同一固定 Lucide 修订，原字节、大小和哈希见 [图标清单](manifests/lucide.json)。

OneSub 在创建 Windows 播放器环境前启用 Chromium 的 `--disable_direct_composition_video_overlays` 驱动兼容项，只停用独立视频叠加平面，保留 GPU 解码、JASSUB 及 MyGo 必需的 DirectComposition 宿主。选项原名由 [`GpuDriverBugList::AppendWorkaroundsFromCommandLine`](https://github.com/chromium/chromium/blob/c6b1ec7e3f83227fa110f2effbd1b1695b22d089/gpu/config/gpu_driver_bug_list.cc) 解析，再由 [`SetProcessGlWorkaroundsFromGpuFeatures`](https://github.com/chromium/chromium/blob/c6b1ec7e3f83227fa110f2effbd1b1695b22d089/gpu/config/gpu_util.cc) 传给视频叠加支持判断；保留下划线名称，不替换为连字符版本。这里只调用已有运行时配置，没有复制 Chromium 代码或引入新依赖。

Windows 11 / WebView2 `158.0.4370.1` 的正式 `OneSub.exe` 对照中，正常 16:9 视频的首个暂停画面被显示为约 1.328:1，位置和尺寸也未铺入原生视口；调整窗口后恢复。仅关闭硬件解码、CSS 绝对定位或 `loadeddata` 强制测量均未修复。启用上述兼容项后，H.264 与 1920×1080/60 fps AV1 的首帧、留边及连续调整尺寸通过真实窗口像素检查。测试使用 `tests/e2e/player_resize.py`，以原生 UI Automation 画面区域核对居中及铺满，不读取网页几何来干扰首帧；改名的同字节程序会掩盖本机异常，正式验收因此要求进程名为 `OneSub.exe`。不能以无错误码或 DOM 尺寸正常代替画面验收。后续更新 WebView2/MyGo 时复核此兼容项，其他 GPU 与混合 DPI 仍须实机验证。

实际引入组件时，先建立 `manifests/`，逐项记录：名称、用途、项目 URL、精确 SHA、下载 URL、文件 SHA-256/字节数、许可证文件、SPDX、版权、全部传递依赖、构建选项及修改范围。未解决项阻止该组件分发，不用空清单冒充已冻结。

- 大型上游源码下载到忽略的 `sources/`，按清单校验；不嵌套完整上游 Git 历史。
- 移植代码放真实消费者源码目录，在 `ports/<组件>/` 记录原路径、符号、SHA、修改、许可副本和验证；搬动路径同步映射。
- OneAsr 的集成补丁在实际开发时放 `patches/oneasr/`，仅限机器协议、目录隔离及产物可靠性等已定边界。
- 字幕技能权利人于 2026-10-27 授权其自有内容以 AGPL-3.0-or-later 随 OneSub 源码及安装器分发。引入时保存完整快照与授权说明，并核查第三方材料；不只复制提示词，也不自行赋予 MIT 许可。
- 模型区分基础模型、转换版本和镜像；字体核查嵌入/再分发权。代码仓库许可证不能自动覆盖权重和字体。
- 第三方许可放 `licenses/`，对应源码与构建配方随未来发行可取得。二进制与大资源放发布资产或下载缓存，不进入 main。

主界面文案统一使用“软字幕（封装）”“硬字幕（烧录）”“背景框”等通行术语，不以自创名称替代；质量参数按真实编码设置展示。维护者提供的应用图标单独登记于 [品牌资源](../apps/desktop/branding/README.md)，不属于 Lucide 操作图标库。

字幕工具栏的设计检查参考 [Impeccable](https://github.com/pbakaus/impeccable)（pbakaus，Apache-2.0）与 [unslop](https://github.com/heydamir/unslop)（Damir Sokolovsky / @heydamir，CC-BY-4.0）的操作分组、间距、文案与克制表达方法；本次仅阅读其公开设计指南，没有复制脚本、组件、图标或其他分发资产，也没有将其加入运行时依赖

硬字幕烧录与独立视频压缩默认使用 CPU `libsvtav1`（抽样感知寻优思路参考 MIT 许可的 [ab-av1](https://github.com/alexheretic/ab-av1)，由 OneSub 在 `apps/desktop/internal/media/compress.go` 以 Go 原生实现，使用 [media-runtime.json](manifests/media-runtime.json) 锁定的 `FFmpeg 9.1` 与 `SVT-AV1 4.3.0`）。可选的 GPU 方案使用同一份 FFmpeg 内置的硬件编码器（`av1_nvenc`、`av1_qsv`、`av1_amf`，不支持时为对应的 `hevc_*`），与 CPU 共用同一条滤镜和流映射管线，不另外分发编码程序；ab-av1 同样通过 FFmpeg 调用这些编码器。FFmpeg 9.1 是 2026-11-22 核查时的最新稳定版；同包更新 ffprobe、依赖、许可和对应源码，没有复制 ab-av1 Rust 代码或引入未登记运行时。

编码使用 [SVT 官方速度预设说明](https://github.com/AOMediaCodec/SVT-AV1/blob/v4.3.0/Docs/CommonQuestions.md)中的 preset 8；样本和最终编码使用相同参数，保留原有画质目标。XPSNR 按[官方文档](https://ffmpeg.org/ffmpeg-filters.html#xpsnr)将原片置于第一输入。FFmpeg 9.1 的 `libavfilter/vf_xpsnr.c` 在高位深路径中将第二输入步长假定为画面宽度，部分 AV1 解码缓冲带行填充，会产生错误负分；评分统一转为 8 位 YCbCr，使用该滤镜正确处理步长的复制路径，实际编码保持 10 位。此兼容项应在升级 FFmpeg 后复核，不能只删除格式转换。新增 HTTP 库依赖按实际 DLL 导入和符号校验；ngtcp2 的对应源码包后来删除了一个构建依赖，清单单独登记与二进制 BUILDINFO 校验值一致的原始构建配方。

音频导出的 M4A/AAC 保持 192 kb/s，显式使用 FFmpeg 原生 AAC 的 `fast` coder。[官方编码器说明](https://ffmpeg.org/ffmpeg-codecs.html#aac)说明该方法适合高码率；FFmpeg 9.1 的默认 `twoloop` 在先静音后有声的 PCM 输入上可持续计算不结束，2 秒与 30 秒样本均复现。使用 `fast` 后通过真实时长和全音轨解码校验。该项只影响独立音频编码，视频合成继续保留已有兼容音轨。

具体许可风险与产品发行门禁由 `manifests/` 和各 `ports/` 登记记录。

OneAsr 的默认对齐器为 Qwen3-ForcedAligner-Ultra；Qwen 中英文热词使用单独下载的 SenseVoiceSmall。随包提供的 Windows DXC 原始二进制与许可登记见 [DXC 集成](ports/dxc/README.md)。
