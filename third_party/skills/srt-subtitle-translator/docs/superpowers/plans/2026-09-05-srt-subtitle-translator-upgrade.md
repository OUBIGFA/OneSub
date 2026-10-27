# SRT Subtitle Translator Upgrade Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use inline execution in this session; do not dispatch subagents. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复技能入口、语言规则、格式校验、Windows 交付流程和评测覆盖，使字幕翻译结果可发现、可复核、可回归。

**Architecture:** 保留现有单脚本命令行入口，先在其中建立统一配置、结构快照和严格模式契约，再按稳定责任拆出小模块。说明文件压缩为入口规则，细节继续按条件放在 references 中。

**Tech Stack:** Markdown、Python 3 标准库、JSON、unittest。

**Spec:** `docs/superpowers/specs/2026-09-05-srt-subtitle-translator-upgrade-design.md`

## Global Constraints

- 不调用机器翻译或第三方翻译服务。
- 输出仍使用输入格式；只有用户明确要求时才转换格式。
- 普通模式允许连续语音范围内重分段，严格模式不允许任何时间边界变化。
- 输出文本使用 UTF-8；源文件允许 UTF-8、UTF-8 BOM、GB18030、UTF-16。
- 不修改其他技能目录的既有未提交内容。
- 所有新增行为先写失败测试，再写最小实现。

---

### Task 1: 建立语言配置与入口边界

**Files:**
- Create: `config/language_profiles.json`
- Create: `agents/openai.yaml`
- Modify: `SKILL.md`
- Modify: `references/edge-cases.md`
- Modify: `references/formats.md`

- [x] 写配置加载和技能触发边界的失败测试，覆盖已有语言、未知语言和只检查时间轴的请求说明。
- [x] 运行测试，确认当前代码尚未提供配置加载契约。
- [x] 增加 JSON 语言配置，至少包含目标语言、字符计算方式、句末标点、阅读速度和最大阅读宽度。
- [x] 删除 `SKILL.md` 顶层 `version`，压缩重复规则，明确排除范围和文件处理模式。
- [x] 添加 UI 元数据，保持隐式触发开启并使用简短默认提示。
- [x] 将 Windows 拼接说明改为 `assemble_subtitle.py` 流程，修正文档中的 `cat` 示例。
- [x] 运行配置和入口相关测试，确认配置字段被正确读取。

### Task 2: 修复格式解析、编码和顺序检查

**Files:**
- Modify: `scripts/check_subtitle.py`
- Test: `scripts/test_check_subtitle.py`

- [x] 先增加失败测试：非法 SRT 编号、时间范围、乱序块、UTF-16 源文件和缺少 UTF-8 输出。
- [x] 运行测试确认这些缺陷可复现。
- [x] 加入 BOM/空字节优先识别，严格验证时间戳、SRT 编号和块顺序；输出文件只接受 UTF-8。
- [x] 让语言配置驱动阅读速度、宽度、计数方式和标点规则，并支持语言地区码回退。
- [x] 运行新增测试和原有测试。

### Task 3: 实现时间轴契约与严格模式

**Files:**
- Modify: `scripts/check_subtitle.py`
- Test: `scripts/test_check_subtitle.py`

- [x] 先增加失败测试：严格模式下合并两块、改变边界、漏掉连续语音和新增间隔都必须失败。
- [x] 运行测试确认当前严格模式错误地接受合并。
- [x] 统一停顿代理阈值，严格模式逐项比较源输出块的数量和时间边界；普通模式只允许源范围内重分段。
- [x] 将检查报告中的“audio contract”改成可证实的字幕间隔代理描述。
- [x] 运行时间轴测试，确认普通模式的合法重分段仍可通过。

### Task 4: 增加 VTT/ASS 结构与标记保真检查

**Files:**
- Modify: `scripts/check_subtitle.py`
- Modify: `references/formats.md`
- Test: `scripts/test_check_subtitle.py`
- Create: `evals/fixtures/structure.vtt`
- Create: `evals/fixtures/structure.ass`

- [x] 先增加失败测试：删除 VTT NOTE/cue 设置、ASS 样式字段或 override tag 时必须失败。
- [x] 运行测试确认当前检查器只验证时间，不验证这些结构。
- [x] 为 VTT/ASS 建立结构快照，比较非对白结构、保护字段、标签和严格模式字段。
- [x] 保留允许的合并策略：普通模式可减少对白块，但不得丢失静态结构或改变输出事件的保护字段。
- [x] 运行格式测试并检查 JSON 输出包含结构错误。

### Task 5: 提取拼接工具并修正文档路由

**Files:**
- Create: `scripts/assemble_subtitle.py`
- Create: `scripts/test_assemble_subtitle.py`
- Modify: `references/edge-cases.md`
- Modify: `SKILL.md`

- [x] 先写失败测试：SRT 分块拼接、VTT 只保留一个头、ASS 只保留一次头部区段。
- [x] 运行测试确认工具不存在。
- [x] 实现 UTF-8 输出、格式识别、块边界校验和跨平台参数。
- [x] 将长文件交付流程改为“拼接→校验→回收自建临时文件”。
- [x] 运行工具测试。

### Task 6: 重建真实评测夹具

**Files:**
- Modify: `evals/evals.json`
- Create: `evals/fixtures/*.srt`
- Modify: `references/segmentation.md`
- Modify: `references/style-common.md`
- Modify: `references/style-zh.md`
- Modify: `references/glossary-3d-zh.md`

- [x] 为核心评测绑定真实输入文件，新增反向触发、严格模式、结构保真、语言配置和 Windows 交付场景。
- [x] 修正“各种变化/随机变化”和 `MoGraph` 规则冲突。
- [x] 删除重复段落，保留每条规则的唯一权威位置。
- [x] 添加中文静态句法启发式的误报样例，并将其定位为提示而非证明。
- [x] 运行 JSON 解析和 fixture 完整性检查。

### Task 7: 最终验证

**Files:**
- Verify: `SKILL.md`
- Verify: `config/language_profiles.json`
- Verify: `scripts/check_subtitle.py`
- Verify: `scripts/assemble_subtitle.py`
- Verify: `scripts/test_check_subtitle.py`
- Verify: `scripts/test_assemble_subtitle.py`
- Verify: `evals/evals.json`

- [x] 运行 `quick_validate.py`。
- [x] 运行全部 unittest，确认失败数为 0。
- [x] 运行 SRT/VTT/ASS 的 CLI 校验矩阵，分别检查普通、严格、双语和多说话人模式。
- [x] 检查 Git diff，仅包含本次技能升级文件和计划文档，不触碰其他技能改动。
