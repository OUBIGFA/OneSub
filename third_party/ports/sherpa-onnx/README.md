# sherpa-onnx 识别引擎集成

核查日期：2026-11-04。OneSub 不复制或修改 sherpa-onnx 与模型权重，只调用官方 CLI 与 C API。

## 固定来源

- sherpa-onnx `v2.0.1`，标签修订 [`af2d8d81dbfa61c4cea4c568da78f3982edca005`](https://github.com/k2-fsa/sherpa-onnx/tree/af2d8d81dbfa61c4cea4c568da78f3982edca005)，Apache-2.0，见[许可](../../licenses/sherpa-onnx.txt)。
- 使用官方发布包 `sherpa-onnx-v2.0.1-win-x64-shared-MT-Release-no-tts.tar.bz2`，SHA-256 已与 GitHub Release digest 核对。只取 5 个文件：`sherpa-onnx-offline.exe`、`sherpa-onnx-offline-language-identification.exe`、`sherpa-onnx-c-api.dll`、`onnxruntime.dll`、`onnxruntime_providers_shared.dll`，合计约 25 MB。字节数与哈希见[运行时清单](../../manifests/runtimes.json) 的 `sherpaOnnx`。
- 包内 onnxruntime `1.28.2`，MIT，见[许可](../../licenses/onnxruntime.txt)。其 ThirdPartyNotices 尚未收集，列入清单的 `remainingMaterials`。

## 模型

模型不随安装器分发，由用户在模型管理中下载。地址固定到下列 Hugging Face 修订，文件大小和 SHA-256 记录在 `apps/desktop/internal/models/catalog_sherpa.json`。Parakeet 的原始模型由 [NVIDIA](https://huggingface.co/nvidia/parakeet-tdt-0.6b-v3) 提供，适用 [CC-BY-4.0](https://creativecommons.org/licenses/by/4.0/)；INT8 与 FP32 是 sherpa-onnx 的 ONNX 转换资产。

| OneSub 模型 ID | 来源仓库与修订 | 原始模型 | 许可 |
| --- | --- | --- | --- |
| `sherpa-sense-voice-small-int8` | `csukuangfj/sherpa-onnx-sense-voice-zh-en-ja-ko-yue-2024-07-17` @ `2365baea` | FunAudioLLM SenseVoiceSmall | FunASR 模型许可 1.1，见[许可](../../licenses/funasr-models.txt) |
| `sherpa-paraformer-zh-int8` | `csukuangfj/sherpa-onnx-paraformer-zh-2023-09-14` @ `def02708` | Paraformer-large 中文 | 同上；转换仓库标注 Apache-2.0 |
| 同上（`punct.onnx`） | `csukuangfj/sherpa-onnx-punct-ct-transformer-zh-en-vocab272727-2024-04-12` @ `432aeba6` | CT-Transformer 中英标点 | FunASR 模型许可 1.1 |
| `sherpa-parakeet-tdt-0.6b-v3-int8` | `csukuangfj/sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8` @ `2bda32ec` | NVIDIA Parakeet-TDT 0.6B v3 | CC-BY-4.0 |
| `sherpa-parakeet-tdt-0.6b-v3-fp32` | `csukuangfj/sherpa-onnx-nemo-parakeet-tdt-0.6b-v3` @ `1a468a35` | NVIDIA Parakeet-TDT 0.6B v3，未量化 ONNX | CC-BY-4.0 |
| `sherpa-whisper-tiny-int8`（语言检测） | `csukuangfj/sherpa-onnx-whisper-tiny` @ `65176e2d` | OpenAI Whisper tiny | MIT，见[许可](../../licenses/whisper.txt) |

FP32 的 `encoder.weights` 是 `encoder.onnx` 的外部数据，不能省略、改名或换目录；5 个文件合计 2,549,800,429 字节，已下载文件与固定修订的大小及 SHA-256 一致。FP32 与 INT8 共用 25 种语言、同一 CLI 和时间转换，不声明 FP32 的识别质量优于 INT8。2026-11-04 核查 `csukuangfj/sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-fp16` @ `abb5c91f` 只有 `.gitattributes`，GitHub `asr-models` 也只有 v3 INT8 资产；其他 FP16 转换仓库使用合并的 `decoder_joint-model.onnx`，不是当前 sherpa-onnx 的 encoder/decoder/joiner 接口。因此 FP16 不列入可选模型，不使用空仓库或不同接口伪造下载渠道。

选型依据（2026-11-04 在本机用各仓库自带录音实测）：

- SenseVoice 的 `2025-09-09` int8 转换版本在 v2.0.1 下把中文识别为粤语、英文输出逐字母大写，弃用；`2024-07-17` int8 版本五种语言结果正确。
- Paraformer `2024-03-09` 转换版本不输出时间戳；`2023-09-14` 版本输出逐字 CIF 时间。两者均不含标点，需要 CT-Transformer。
- 语言检测：Whisper tiny 对 7 段自带录音的整段结果中，粤语判为 zh，其余正确；截成 1.5 秒时粤语判为 ja。sherpa-onnx 的检测接口只返回最可能的语言。因此采用 10 秒窗口多数表决，中日韩由 SenseVoiceSmall 复核，见 [架构 4.4](../../../docs/architecture.md)。
- CT-Transformer 只需 `model.onnx`（词表已在模型元数据内），不下载 `tokens.json`。

## 适配边界

实现位于 `apps/desktop/recognition_sherpa.go`、`punctuate_windows.go`；token 转词与断句位于 `internal/subtitle/sherpa.go`。

1. 语音检测、静音保护与 OneAsr 路径共用同一 FireRedVAD；输入不超过 20 秒，相邻输入不重叠。
2. CLI 以 ANSI 代码页读取参数。所有 WAV 与模型路径都是工作目录内的 ASCII 相对路径，模型目录经目录联接挂载，不复制权重。
3. CT-Transformer 的 CLI 会丢失中文，因此由 `OneSub.exe punctuate` 子进程调用 C API，文本以 UTF-8 JSON 行传递。该子进程先按路径加载包内 onnxruntime，避免绑定 Windows 11 System32 中的 1.17 版本（实测会因 API 版本不符崩溃）。
4. 每批 16 个输入一次 CLI 运行；结果条数与输入不符、token 与时间数量不符均为协议错误，不回退成整段识别。
5. Parakeet-TDT 常把每个输入首个 token 的时间报为 0。输入前带最多 250 ms 的语音检测上下文，所以每段语音的第一个词最多提前 250 ms 开始；不改写模型给出的时间。
6. 只用 CPU。官方另有 CUDA 包，体积约 0.5 GB，未纳入。

## 构建与验证

- `build/prepare-sherpa.ps1` 按清单取得或校验发布包，只解出 5 个文件到 `build/output/sherpa-onnx/`；`build/build-windows.ps1` 自动调用。
- `build/test.ps1` 设置 `ONESUB_SHERPA_ROOT`。再把 `ONESUB_SHERPA_TEST_MODELS` 指向按模型 ID 分目录的模型根目录（含 16 kHz 的 `test_wavs/zh.wav`、`en.wav`、`paraformer.wav`），即运行真实推理回归：四个识别模型各识别自带录音；SenseVoice、Parakeet INT8 与 FP32 另跑“语音 + 40 秒静音 + 语音”与全静音。自动识别回归对中、粤、日、韩、英、德 6 段录音检测并路由，另测首选模型缺失时的保底与报错、检测模型缺失时的报错。测试不下载模型。
- 仓库自带录音只验证链路，不代表识别质量。中文、英文、粤语、日语、韩语与欧洲语言的 CER/WER、时间误差与静音幻觉仍需按 [实施与验收](../../../docs/roadmap.md) 建立评测集。
