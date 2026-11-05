<div align="center">

<img src="apps/desktop/branding/logo.png" width="160" alt="OneSub Logo">

# OneSub

Local-first subtitle workbench: recognize, split, translate, edit, compress, and burn in

[简体中文](README.md) | English

![Platform](https://img.shields.io/badge/platform-Windows%2010%2F11%20x64-555555) ![WebView2](https://img.shields.io/badge/WebView2-runtime-555555) ![License](https://img.shields.io/badge/license-AGPL--3.0-555555)

</div>

![OneSub workbench: original subtitles](assets/readme/workbench-original.webp)

![OneSub workbench: rebuilt subtitles with translation](assets/readme/workbench-translated.webp)

## Rebuild timing by sense groups: before and after

Enable "Rebuild timing by sense groups" to merge isolated words and short fragments into complete sense groups. OneSub then adjusts cue boundaries from the source timing. Complete thoughts help the model translate more accurately.

![Sense-group timing comparison: disabled on the left, enabled on the right](assets/readme/sense-group-comparison.webp)

**Left: disabled. Right: enabled.** On the left, `equally.` has its own cue and appears for only 660 milliseconds. On the right, it joins the previous sentence. The complete cue runs from `00:00:27,880` to `00:00:31,820`. OneSub also joins `Next,` to the instruction that follows.

OneSub saves the result in the rebuilt track and keeps the original track separate. With word-level timing, it aligns boundaries to real words. Without word-level timing, it only merges existing cues. We strongly recommend this option with `gpt-6-luna` or a model of equal or greater capability.

## Video demo

https://github.com/user-attachments/assets/64fed7da-2e36-48a5-8409-68cfbcf6a299

Demo conditions: 42-minute source video; Qwen3-ASR 1.7B recognition model; `gpt-6-luna` AI model; "Rebuild timing by sense groups" enabled. Recognition and translation took 6 minutes in this run. The demo runs at increased speed. Actual time depends on hardware, network, and service.

## What OneSub is

OneSub imports media, recognizes speech, translates, edits, previews, and exports subtitles in one workbench. Traditional subtitle translators translate the source lines as they are. Source lines often cut one sentence into several cues. The translation keeps these cuts, and the timeline goes wrong. OneSub regroups the dialogue into sense groups first. It then aligns the text to word-level time evidence. The original track stays separate from the rebuilt track.

Recognition, editing, and export need no cloud account. Translation uses the AI service that you configure.

## Key features

- **Smart split**: Split long cues in one click, or use the context menu to split one cue. OneSub selects cut points from the text language. In the rebuilt track, the translation controls the split. OneSub uses real word-level timing when available. Otherwise, it estimates cut points within the original range and marks them for timing review. A compact sentence with no valid cut point stays unchanged. Undo a batch split in one step.
- **Punctuation handling**: OneSub processes punctuation locally after translation. It adds no request and changes no prompt. You can remove or keep final punctuation. You can convert `.` inside a sentence to `,` and `。` to `，`. Email addresses, URLs, numbers, abbreviations, file names, and ellipses are protected. Recognition output keeps its original punctuation.
- **Rebuild timing by sense groups**: Merge isolated words and short fragments into complete sense groups before translation. Align boundaries to real words when word-level timing is available. Otherwise, only merge existing cues.
- **Separate tracks**: Original, rebuilt, and translated tracks are independent. Edit, export, and undo each one (at least 200 steps). AI does not overwrite locked manual edits.
- **Subtitles alone**: No video needed. Import SRT / VTT / ASS / SSA to edit, translate, and export. ASS keeps its layout. Choose TXT to export plain text without cue numbers, timestamps, or styling.
- **Video compression alone**: No subtitle needed. OneSub encodes H.264 into MP4 or MKV, keeps audio and chapters, and never overwrites the source video.
- **Multi-engine local recognition**: SenseVoiceSmall, Paraformer-zh, Parakeet-TDT v3, and Qwen3-ASR. With source language set to "Auto-detect", OneSub detects the language first, then selects the matching model. Models download on demand with resume and SHA-256 checks.
- **Silence protection**: VAD filters silence first. Long silence makes no false cues.
- **File queue**: Transcribe many media files in a batch. Retry failed items alone.
- **Controlled translation**: Full-video context, locked terms, and per-cue checks. A missing translation is never filled with the source text. Negation, numbers, units, and literal strings are protected.
- **Resumable**: After an interruption, OneSub keeps the finished batches and continues from the missing ones.
- **What you see is what you get**: Preview and burn-in share one subtitle, style, and font configuration. Both soft subtitles (muxed) and hard subtitles (burned in) are supported.
- **Auto link**: Drop a video to load its same-name subtitle. Drop a subtitle to link its same-name video. Language suffixes (such as `-zh.srt`) and small name differences are scored. You choose when several candidates match.
- **Chinese and English interface**, light and dark themes.

## Differences from traditional subtitle translators

| Item | Traditional tools | OneSub |
| --- | --- | --- |
| Segmentation | Keeps source segments | Regroups by sense group; cuts fall on word boundaries |
| Timeline | Inherited per line; translation drifts from speech | Aligned to word-level time evidence |
| Original subtitle | Overwritten by the translation | Original, rebuilt, and translated tracks stay separate |
| Failed translation | Often filled with the source text | Never filled; missing items are retried and flagged |
| Manual edits | Lost on re-translation | Locked content is never overwritten by AI |
| Speech recognition | Cloud service or one model | Local multi-engine with language detection |
| Silence | May produce hallucinated cues | VAD filter; no cues in long silence |
| Punctuation | Inconsistent in translation | Local rules; protected items stay unchanged |
| Preview vs. output | Styles differ | Shared configuration |
| Workflow | Create a project, then move through several pages | Drop a file and play; recognize, translate, and burn in on one workbench |
| Privacy | Often uploads media | Recognition runs on your computer; only subtitle text goes to the AI service you choose |

## Download

Download `OneSub-0.0.1-Setup.exe` from [Releases](https://github.com/OUBIGFA/OneSub/releases). Check the SHA-256 on the same page, then run it.

- Version 0.0.1, Windows x64 only. The installer is unsigned. Windows may show a security prompt.
- The installer is about 256 MB. Recognition models download on demand. They range from several hundred MB to several GB. You can change the location.

## System requirements

- Windows 11, or a verified Windows 10 x64
- WebView2 runtime (the installer installs it)
- Recognition currently runs on the CPU

## Quick start

1. Drop a video into the window. It plays at once.
2. Click "One-click transcribe & translate". On first use, follow the prompts to choose a recognition model and set up an AI service.
3. Edit in the subtitle list and the edit panel.
4. Click "Export subtitles", or click "Compose video".

Subtitles only: drop the subtitle file, then edit and translate. Video only: click "Compress video".

## Recommended settings

| Item | Value | Reason |
| --- | --- | --- |
| Video encoding | CRF 23 | Small file, enough quality for most videos |
| Translation concurrent batches | 2 to 5 | A higher number may hit service rate limits |
| Cues per batch | 50 | Balances context and stability |
| Rebuild timing by sense groups | Strongly recommended with a model at or above gpt-6-luna | More reliable translation and a better timeline |

A weaker model may fail to return stable segmentation. Turn the option off in that case.

## Get gpt-6-luna usage with a ChatGPT account

1. Register a ChatGPT account and sign in to Codex.
2. Proxy that sign-in with [CLIProxyAPI (CPA)](https://github.com/router-for-me/CLIProxyAPI) or [magpie](https://github.com/yetone/magpie-releases). You get a local OpenAI-compatible address.
3. In OneSub settings, add an AI service. Enter that address and select the model `gpt-6-luna`.

OneSub supports the OpenAI Chat Completions and Responses protocols. CPA and magpie are third-party projects and are not affiliated with OneSub. OpenAI decides the available quota, limits, and terms. They can change at any time. Read the terms before you use them.

## License and credits

This project is released under **AGPL-3.0-or-later**. See [LICENSE](LICENSE). Third-party code, fonts, and models follow their own licenses. They are registered in the [third-party rules](third_party/README.md).
