# 第三方来源与许可规则

当前已引入 MyGo、官方 MCP Go SDK、libse worker、SmartSub 样式模块、OneAsr 模型目录与开发运行时、完整字幕技能快照、Lucide 图标以及 JASSUB 播放渲染依赖。识别前的静音保护直接复用 OneAsr 固定的 FireRedVAD `v1.0.0` 模型与默认设置，见 [集成登记](ports/oneasr/README.md)。SenseVoice、Paraformer 与 Parakeet-TDT 由 Apache-2.0 的 sherpa-onnx `v2.0.1` 官方 CPU 运行时执行，模型由用户按固定修订下载，不随安装器分发，见 [集成登记](ports/sherpa-onnx/README.md)。精确记录位于 `manifests/`，移植边界位于 `ports/`。[研究证据](../docs/research/evidence.json)仍只用于追溯设计阅读，不替代实际依赖锁。

当前安装器仅用于本地测试。`manifests/runtimes.json` 明确记录尚未补齐的传递许可、对应源码材料、签名与干净机器验收；这些门禁未过前不对外发布。JASSUB 的包装层 MIT 不代表其 WASM 只有 MIT 许可。

文件名分级关联直接调用 MIT 的 `agnivade/levenshtein v1.2.1` 计算 Unicode 编辑距离，规范化与语言代码解析复用既有 `golang.org/x/text`。精确修订、模块／下载校验、许可与测试专用依赖见 [文件名匹配清单](manifests/filename-matching.json)。不复制上游算法，不采用 Subtitle Edit 按首个文件返回或任意截短名称的匹配实现。

字幕阅读负荷与智能拆分的上游修订、实际阅读符号及采用／拒绝的算法边界见[调研记录](../docs/research/subtitle-readability.md)。没有复制 Subtitle Edit、stable-ts 或 VideoCaptioner 的拆分代码。中文直接筛选 MIT 的 jieba 固定词典并编译为嵌入式双数组 Trie；日文／泰文移植 Apache-2.0 BudouX 的纯文本评分并保留原模型，见[集成登记](ports/subtitle-splitting/README.md)及[固定清单](manifests/subtitle-splitting.json)。没有新增 CGO、Python 或网络运行时；字典和模型以可复现 UTF-8 文本嵌入，不扩大仓库二进制例外。Unicode 字素／词界直接调用现有 `Unlicense OR BSD-3-Clause` 的 `go-text/typesetting v0.3.5`，字符宽度使用既有 `golang.org/x/text v0.43.0`。字幕技能的阅读负荷阈值由轻量本地检测使用，许可沿用完整技能快照登记。

实际翻译协议与分段流程参考的 VideoCaptioner、SmartSub 固定修订、源码位置和适配边界见 [组件复用核查第 7 节](../docs/research/implementation-reuse.md#7-翻译协议容错与分段参考)。它们不是新增运行时依赖；完整字幕技能快照仍为质量规则来源。

界面层级参考 Impeccable 的公开设计说明，仅作为阅读证据，不复制或分发该项目技能与代码，见 [界面规范](../docs/ui-design.md)。浅色主题直接复用 WinOne 固定修订的颜色值，来源、许可副本、哈希与有限适配边界见 [WinOne 配色登记](ports/winone/README.md)；没有引入其控件、绘制代码或运行时。

MyGo 原生 UI 当前锁定为 `v0.7.4`；依赖、源码 ZIP 与许可校验见 [运行时清单](manifests/runtimes.json)，有限补丁与行为边界见 [补丁登记](patches/mygo/README.md)。已移除上游具备的复合输入占位转发与菜单附加宽度算法，直接复用对应原生能力。按项目工作约定持续关注并及时同步上游最新版本；升级时重新核对 API、补丁必要性、许可与相关原生验收，当前锁定修订只用于可复现构建。

实际引入组件时，先建立 `manifests/`，逐项记录：名称、用途、项目 URL、精确 SHA、下载 URL、文件 SHA-256/字节数、许可证文件、SPDX、版权、全部传递依赖、构建选项及修改范围。未解决项阻止该组件分发，不用空清单冒充已冻结。

- 大型上游源码下载到忽略的 `sources/`，按清单校验；不嵌套完整上游 Git 历史。
- 移植代码放真实消费者源码目录，在 `ports/<组件>/` 记录原路径、符号、SHA、修改、许可副本和验证；搬动路径同步映射。
- OneAsr 的集成补丁在实际开发时放 `patches/oneasr/`，仅限机器协议、目录隔离及产物可靠性等已定边界。
- 字幕技能权利人于 2026-10-27 授权其自有内容以 AGPL-3.0-or-later 随 OneSub 源码及安装器分发。引入时保存完整快照与授权说明，并核查第三方材料；不只复制提示词，也不自行赋予 MIT 许可。
- 模型区分基础模型、转换版本和镜像；字体核查嵌入/再分发权。代码仓库许可证不能自动覆盖权重和字体。
- 第三方许可放 `licenses/`，对应源码与构建配方随未来发行可取得。二进制与大资源放发布资产或下载缓存，不进入 main。

主界面文案统一使用“软字幕（封装）”“硬字幕（烧录）”“背景框”等通行术语，不以自创名称替代；质量参数按真实编码设置展示。维护者提供的应用图标单独登记于 [品牌资源](../apps/desktop/branding/README.md)，不属于 Lucide 操作图标库。

具体许可风险与产品发行门禁见 [安全与分发](../docs/security-and-release.md)。
