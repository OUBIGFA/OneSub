# 第三方来源与许可规则

工作台组件与主题使用 MIT 的 [Lobe MyGo](patches/lobe-mygo/README.md)，底层窗口、布局、虚拟表格与媒体承载使用 MyGo。旧版 WinOne 和 Fluent UI 设计来源记录保留用于追溯，不再作为当前界面的主题实现。

当前已引入 MyGo、官方 MCP Go SDK、libse worker、SmartSub 样式模块、OneAsr 模型目录与开发运行时、完整字幕技能快照、Lucide 图标以及 JASSUB 播放渲染依赖。识别前的静音保护直接复用 OneAsr 固定的 FireRedVAD `v1.0.0` 模型与默认设置，见 [集成登记](ports/oneasr/README.md)。SenseVoice、Paraformer 与 Parakeet-TDT 由 Apache-2.0 的 sherpa-onnx `v2.0.1` 官方 CPU 运行时执行，模型由用户按固定修订下载，不随安装器分发，见 [集成登记](ports/sherpa-onnx/README.md)。精确记录位于 `manifests/`，移植边界位于 `ports/`。

当前安装器仅用于本地测试。`manifests/runtimes.json` 明确记录尚未补齐的传递许可、对应源码材料、签名与干净机器验收；这些门禁未过前不对外发布。JASSUB 的包装层 MIT 不代表其 WASM 只有 MIT 许可。

TXT 文稿导出直接调用固定 libse 的 `HtmlUtil.RemoveHtmlTags`、`HtmlUtil.RemoveOpenCloseTags`、`Utilities.RemoveSsaTags` 与 `AdvancedSubStationAlpha.RemoveDrawingTag`；没有复制上游导出界面或引入新的运行时。字幕 worker 的定时写入与格式保护保持不变。

文件名分级关联直接调用 MIT 的 `agnivade/levenshtein v1.2.1` 计算 Unicode 编辑距离，规范化与语言代码解析复用既有 `golang.org/x/text`。精确修订、模块／下载校验、许可与测试专用依赖见 [文件名匹配清单](manifests/filename-matching.json)。不复制上游算法，不采用 Subtitle Edit 按首个文件返回或任意截短名称的匹配实现。

字幕阅读负荷与智能拆分的算法边界见 `ports/subtitle-splitting/README.md`。没有复制 Subtitle Edit、stable-ts 或 VideoCaptioner 的拆分代码。中文直接筛选 MIT 的 jieba 固定词频／词类词典并编译为嵌入式双数组 Trie，结合 Apache-2.0 BudouX 0.10.3 简繁中文模型的边界评分；日文／泰文保留既有 BudouX 模型。英文直接引用 MIT 的 prose 3.2.3 分词与词性模型，上游明确授权模型按 MIT 分发。见[集成登记](ports/subtitle-splitting/README.md)、[调研记录](ports/subtitle-splitting/RESEARCH.md)及[固定清单](manifests/subtitle-splitting.json)。没有新增 CGO、Python 或网络运行时；中文字典和模型以可复现 UTF-8 文本嵌入，英文模型通过 Go 模块嵌入，不扩大仓库二进制例外。Unicode 字素／词界直接调用现有 `Unlicense OR BSD-3-Clause` 的 `go-text/typesetting v0.3.5`，字符宽度使用既有 `golang.org/x/text v0.43.0`。字幕技能的阅读负荷阈值由轻量本地检测使用，许可沿用完整技能快照登记。

实际翻译协议与分段流程参考的固定上游修订、源码位置和适配边界见各组件的 `ports/` 登记。它们不是新增运行时依赖；完整字幕技能快照仍为质量规则来源。

旧版浅色主题的 WinOne 来源、许可副本、哈希与适配边界见 [历史配色登记](ports/winone/README.md)；当前浅深色主题均使用 Lobe MyGo 的主题令牌。

MyGo 原生 UI 当前锁定为 `v0.8.4-0.20261110112535-a91f511e89c9`；依赖、源码 ZIP 与许可校验见 [运行时清单](manifests/runtimes.json)，有限补丁与行为边界见 [补丁登记](patches/mygo/README.md)。2026-11-10 核查时最新产品发布为 0.8.3；Lobe MyGo 依赖后续原生能力，因此使用核查时最新兼容主分支修订。升级时重新核对 API、补丁必要性、许可与相关原生验收，当前锁定修订只用于可复现构建。

MyGo 的应用更新直接复用 `mygo.Updater.Check`／`Update.Install`，包括 HTTPS／系统代理、Ed25519 校验、解包与替换／回滚；没有引入另一套下载器或安装器。设置“关于”与任务协调由应用原生 UI 承担，产品版本保持 `0.0.1`，用数值第四段区分测试构建。归档与签名由 MyGo 构建器生成，只调整清单中的内部构建号与不可变 Release URL，见 [构建与更新说明](../build/README.md)。新增 GitHub 和“关于”图标继续直接复用同一固定 Lucide 修订，原字节、大小和哈希见 [图标清单](manifests/lucide.json)。

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

具体许可风险与产品发行门禁由 `manifests/` 和各 `ports/` 登记记录。
