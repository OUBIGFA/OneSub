# OneAsr 与非语音过滤集成

核查日期：2026-11-13。OneSub 复用固定识别内核，只补任务内批片段调用，不修改解码器、模型、语音过滤或时间算法。

## 固定来源

- OneAsr `v1.3.0`，源码修订 [`9f43a8b390d503fc1ff0122cb17561032d5e6e22`](https://github.com/eclipse005/OneAsr/tree/9f43a8b390d503fc1ff0122cb17561032d5e6e22)。运行时由该源码与[批片段补丁](../../patches/oneasr/README.md)构建；来源、源码和补丁校验见[集成清单](../../manifests/oneasr.json)，实际二进制哈希见[运行时清单](../../manifests/runtimes.json)。
- OneAsr 的 `Cargo.lock` 固定 FireRedVAD `v1.0.0`，修订 [`1d0d349464635f33123af18fd4d373efc4613f26`](https://github.com/eclipse005/FireRedVad/tree/1d0d349464635f33123af18fd4d373efc4613f26)。OneSub 使用该版本的官方 Windows x64 CLI，未修改二进制，仅命名为 `fireredvad.exe`。
- FireRedVAD 代码为 Apache-2.0，见 [许可](../../licenses/fireredvad.txt)。该版本 README 明确说明嵌入的 VAD、AED、Stream-VAD 权重来自 FireRedTeam/FireRedASR2S 并以 Apache-2.0 再分发；另保留 [权重来源许可](../../licenses/fireredvad-models.txt)。OneSub 调用同一二进制内的 VAD 和 AED，不另下载权重。
- `fireredvad.exe` 为 9,353,216 字节；SHA-256 `ea991bb111efe7984c72fcaf31e21717a2ac8a08dbc8c26695a60d7b4ce2dd69`，已与 GitHub Release digest 核对。

## 已复用的参数和防护

| 位置 | 上游行为 | OneSub 集成 |
| --- | --- | --- |
| `oneasr-core/src/settings.rs` | 分块默认 60 秒，范围 30–180；最大生成 Token 默认 2048 | 沿用默认值，并传递用户当前设置 |
| `oneasr-core/src/engine/local/qwen_asr.rs` | `TranscribeOptions::default()`，设置源语言与最大生成 Token | 直接运行同一 CLI/core，无自建解码器 |
| `qwen3-asr-wgpu` 修订 `28d4ec7510a057d50273af340de228a145d34e23`，`src/prompt.rs` | 解析输出时以阈值 20 清理异常字符/模式重复 | 已包含于固定 OneAsr 二进制；不是可保证消除幻觉的重复惩罚参数 |
| `oneasr-core/src/vad.rs` | FireRedVAD 的 `VadConfig::default()`，用静音位置选择连续分块 | 保留 core 的分块逻辑；所有模型的输入先经过 VAD 候选和 AED 说话确认 |
| FireRedVAD `src/aed.rs` | 同时输出 `speech`、`singing`、`music` 的时间段；三类可以重叠 | 使用 `AedConfig::default()` 的 `speech` 时间轴确认说话，不因存在背景音乐而删除解说 |

原版 `plan_chunks` 仍覆盖 `0..duration`，且短于分块目标的音频不执行该 VAD 路径。因此“上游已有 VAD”并不意味着无声区间已从识别输入中排除。

VAD 的二类判断还会把歌声和类似人声的音乐当成候选语音。固定上游 [README](https://github.com/eclipse005/FireRedVad/blob/1d0d349464635f33123af18fd4d373efc4613f26/README.md) 推荐直接使用 AED 的 `speech`，或组合 VAD 和 AED。OneSub 参考其 [组合示例](https://github.com/eclipse005/FireRedVad/blob/1d0d349464635f33123af18fd4d373efc4613f26/examples/pipeline_vad_aed.rs) 和 [AED 实现](https://github.com/eclipse005/FireRedVad/blob/1d0d349464635f33123af18fd4d373efc4613f26/src/aed.rs)，采用后者以保留较弱的起始音。

每个未加上下文的 VAD 候选至少有一半时长须被 AED 标为说话，才保留其完整边界。确认在合并和补边之前执行，短词不会因补边而降低说话占比，被拒绝的候选也不能通过相邻段合并重新进入识别。与上游示例选“占比最高类别”不同，OneSub 只核对说话占比：音乐和说话可同时覆盖同一片段，音乐更长不代表解说无效。没有按文字删除重复词或语气词的黑名单。

## 应用适配边界

实现位于 `apps/desktop/pipeline.go`、`recognition_audio.go`，词级文件的解析/序列化共用 `internal/subtitle/oneasr.go`：

1. 使用包内 FFmpeg 提取 16 kHz 单声道 PCM，全部输入先经过 FireRedVAD 的 VAD 和 AED，包括短音频。两个检测器分别对完整 WAV 运行一次。
2. 使用上游默认检测参数。确认说话后，仅合并不超过 1 秒的句内停顿，并在片段前后保留最多 250 ms 的上下文；长静音保持为间隙。
3. 每个通过过滤的片段仍单独提取，不拼接音频。一个进程先加载一次 ASR 处理全部片段，释放后再加载一次对齐模型处理全部片段；每段分别导出。字幕与词级时间沿用原有逻辑，只加一次片段偏移；修正零时长字幕时不延伸到片段外。
4. 未确认说话的静音、音乐或其他声音显示“未检测到语音，未生成字幕”，不调用 ASR 或后续翻译。检测失败、缺失 `speech`、无效时间段、两份结果时长不一致和取消均停止，不回退成单个检测器或整段识别。
5. CLI 子进程的 PATH 优先使用包内媒体工具，不修改用户的系统 PATH。
6. 原 CLI 无识别中途断点接口：识别阶段中断后重新运行识别；识别已经完成时，继续处理跳过识别，翻译只补齐尚未完成的字幕。

同一确认入口同时用于 Qwen、SenseVoice、Paraformer、Parakeet 及自动语言检测。桌面、批处理和 MCP 共用识别管线，不提供绕过确认的模型选项。

检测器输出仍是概率判断，不能作为“所有素材零幻觉”的证明。弱人声、唱词、重叠对白和启用人声分离的复杂输入仍需质量评测；只被判为歌声的输入不会作为说话处理。连续语音内的模型幻觉仍可能发生。任务内复用仅涉及 ASR 和对齐模型；可选人声分离保留逐片段处理，不跨文件常驻。

## 构建与验证

- `build/build-oneasr.ps1` 校验固定源码与补丁，使用 Rust 1.96.0 和原 Cargo.lock 构建 CLI。有效二进制直接复用；首次构建或输入变化时执行 core 格式、Clippy、回归测试及 release 构建，并登记实际 SHA-256。
- `build/prepare-vad.ps1` 按清单校验或取得官方二进制；也可用 `-Source` 指定已下载文件。错误哈希阻止打包。`build/build-windows.ps1` 自动调用此步骤。
- `build/test.ps1` 使用真实 FireRedVAD/FFmpeg 验证 0.1、2、45 秒全静音及合成的“重复语气词 + 配乐解说”；覆盖两个检测结果的结构、重叠类别、短词、被拒片段、边界、词级时间回写和失败/取消。
- 将 `ONESUB_ASR_TEST_MODELS` 指向已有模型根目录，运行同一测试入口，可启用 Qwen3-ASR 1.7B + Qwen 强制对齐集成回归：6 秒语音片段、40 秒静音、同一语音片段。检查每套模型仅加载一次、前后都有字幕、静音内无字幕/词，且后半段不提前。此测试不下载模型、不访问云服务。
- 可选的 `ONESUB_NONSPEECH_TEST_AUDIO` 指向本地 16 kHz 单声道 PCM 非语音录音。检测入口和每个已安装的 ASR 模型均须在推理/发布前停止；素材不入库。`ONESUB_SPEECH_MUSIC_TEST_AUDIO` 与 `ONESUB_SPEECH_END_MS` 接受说话后接音乐的本地样例及独立确认的说话终点，实际运行 Qwen3-ASR 1.7B、SenseVoice、Parakeet，检查保留说话、词证据匹配且音乐尾段没有输出。
- `tests/e2e/process_media.ps1` 使用独立数据和 IPC 目录，在真实工作台验证一键识别翻译、取消与继续处理。
- 最终包内证据与安装器校验值保存在本地验收材料中，不随仓库发布。
