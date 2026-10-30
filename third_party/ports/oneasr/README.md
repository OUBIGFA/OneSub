# OneAsr 与静音保护集成

核查日期：2026-10-30。OneSub 不复制或重写识别内核。

## 固定来源

- OneAsr `v1.3.0`，源码修订 [`9f43a8b390d503fc1ff0122cb17561032d5e6e22`](https://github.com/eclipse005/OneAsr/tree/9f43a8b390d503fc1ff0122cb17561032d5e6e22)。运行时沿用官方 `oneasr-cli.exe`；哈希见 [运行时清单](../../manifests/runtimes.json)。
- OneAsr 的 `Cargo.lock` 固定 FireRedVAD `v1.0.0`，修订 [`1d0d349464635f33123af18fd4d373efc4613f26`](https://github.com/eclipse005/FireRedVad/tree/1d0d349464635f33123af18fd4d373efc4613f26)。OneSub 使用该版本的官方 Windows x64 CLI，未修改二进制，仅命名为 `fireredvad.exe`。
- FireRedVAD 代码为 Apache-2.0，见 [许可](../../licenses/fireredvad.txt)。该版本 README 明确说明嵌入的 VAD、AED、Stream-VAD 权重来自 FireRedTeam/FireRedASR2S 并以 Apache-2.0 再分发；另保留 [权重来源许可](../../licenses/fireredvad-models.txt)。OneSub 仅调用 VAD，不另下载权重。
- `fireredvad.exe` 为 9,353,216 字节；SHA-256 `ea991bb111efe7984c72fcaf31e21717a2ac8a08dbc8c26695a60d7b4ce2dd69`，已与 GitHub Release digest 核对。

## 已复用的参数和防护

| 位置 | 上游行为 | OneSub 集成 |
| --- | --- | --- |
| `oneasr-core/src/settings.rs` | 分块默认 60 秒，范围 30–180；最大生成 Token 默认 2048 | 沿用默认值，并传递用户当前设置 |
| `oneasr-core/src/engine/local/qwen_asr.rs` | `TranscribeOptions::default()`，设置源语言与最大生成 Token | 直接运行同一 CLI/core，无自建解码器 |
| `qwen3-asr-wgpu` 修订 `28d4ec7510a057d50273af340de228a145d34e23`，`src/prompt.rs` | 解析输出时以阈值 20 清理异常字符/模式重复 | 已包含于固定 OneAsr 二进制；不是可保证消除幻觉的重复惩罚参数 |
| `oneasr-core/src/vad.rs` | FireRedVAD 的 `VadConfig::default()`，用静音位置选择连续分块 | 保留 core 的分块逻辑；在输入前再用同版本 VAD 选择有语音的范围 |

原版 `plan_chunks` 仍覆盖 `0..duration`，且短于分块目标的音频不执行该 VAD 路径。因此“上游已有 VAD”并不意味着无声区间已从识别输入中排除。

## 应用适配边界

实现位于 `apps/desktop/pipeline.go`、`recognition_audio.go`，词级文件的解析/序列化共用 `internal/subtitle/oneasr.go`：

1. 使用包内 FFmpeg 提取 16 kHz 单声道 PCM，全部输入先经过 FireRedVAD，包括短音频。
2. 使用上游默认语音检测设置。仅合并不超过 1 秒的句内停顿，并在片段前后保留最多 250 ms 的上下文；长静音保持为间隙。
3. 每个有语音的片段分别进入原版 OneAsr。字幕与词级时间只加一次片段偏移，恢复为原媒体时间；修正零时长字幕时不延伸到片段外。
4. 全静音显示“未检测到语音，未生成字幕”，不调用 ASR 或后续翻译。检测失败、无效 JSON 和取消均不回退成整段识别。
5. CLI 子进程的 PATH 优先使用包内媒体工具，不修改用户的系统 PATH。
6. 原 CLI 无识别中途断点接口：识别阶段中断后重新运行识别；识别已经完成时，继续处理跳过识别，翻译只补齐尚未完成的字幕。

不把 VAD 输出当作绝对正确的语音事实。弱人声、背景音乐、歌声、重叠对白和启用人声分离的复杂输入仍需质量评测。连续语音内的模型幻觉仍可能发生；逐片段启动模型也会增加初始化开销。

## 构建与验证

- `build/prepare-vad.ps1` 按清单校验或取得官方二进制；也可用 `-Source` 指定已下载文件。错误哈希阻止打包。`build/build-windows.ps1` 自动调用此步骤。
- `build/test.ps1` 使用真实 FireRedVAD/FFmpeg 验证 0.1、2、45 秒全静音；覆盖语音区间校验、短停顿、边界、词级时间回写和失败/取消。
- 将 `ONESUB_ASR_TEST_MODELS` 指向已有模型根目录，运行同一测试入口，可启用 Qwen3-ASR 1.7B + Qwen 强制对齐集成回归：6 秒语音片段、40 秒静音、同一语音片段。检查前后都有字幕，静音内无字幕/词，且后半段不提前。此测试不下载模型、不访问云服务。
- `tests/e2e/process_media.ps1` 使用独立数据和 IPC 目录，在真实工作台验证一键识别翻译、取消与继续处理。
- 最终包内证据与安装器校验值记入 [Windows 验收记录](../../../docs/verification/windows-0.0.1.md)。这些局部证据不构成完整 P0 或公开发行放行。
