<div align="center">

![OneSub Logo](apps/desktop/branding/logo.png)

# OneSub

Local-first subtitle workbench

[简体中文](README.md) | English

![Platform](https://img.shields.io/badge/platform-Windows%2010%2F11-555555) ![Go](https://img.shields.io/badge/Go-1.x-555555) ![WebView2](https://img.shields.io/badge/WebView2-runtime-555555) ![License](https://img.shields.io/badge/license-AGPL--3.0-555555)

</div>

OneSub imports media, recognizes speech, translates, previews, and exports subtitles in one workbench. Most subtitle tools translate the source lines as they are. Source lines often cut one sentence into several cues. The translation keeps these cuts, and the timeline stays wrong. OneSub regroups the dialogue into sense groups first. It then aligns the text to word-level time evidence. The result is a rebuilt track with a better timeline. The original track stays separate. Recognition, editing, and export do not need a cloud account.

## Features

- Drop a video to load the same-name subtitle in the same folder; drop a subtitle to link the same-name video; select once when several candidates exist
- Local speech recognition: models download on demand; VAD filters silence first, and long silence makes no false cues; the file queue transcribes in batch, and failed items retry alone
- Sense-group timeline rebuild: continuous dialogue regroups into complete sentences, and the text aligns to word-level time evidence; cues without word-level evidence only merge, and OneSub invents no times
- Controlled translation: full-film context, locked terms, and per-cue checks; missing translations do not fall back to the source text; negations, numbers, units, and literal strings stay protected
- Separate original, rebuilt, and translation tracks: AI does not overwrite manual edits, and locks and undo stay traceable
- Preview and composition share the same subtitle, style, and font settings; soft subtitles (mux) and hard subtitles (burn-in) are both supported
- Chinese and English interface, light and dark theme

## Sense-group timeline rebuild

Traditional tools translate strictly line by line. Source subtitles often split one sentence into several cues by pause or length. The translation follows these cuts, and the cut points land inside words.

OneSub does this:

1. With rebuild on, continuous dialogue regroups into complete sentences; the model only selects source ranges and writes no new timecodes
2. The text aligns back to word-level time evidence, and cut points land on real word boundaries
3. Cues without word-level evidence only merge; OneSub fabricates no times by character ratio
4. The original track and the rebuilt track stay separate; you edit, export, and undo each one alone

## Download

This is the Windows x64 0.0.1 local test build. It is not released to the public. The installer is `OneSub-0.0.1-Setup.exe` and is unsigned.

Tested and untested scope is in the [Windows test record](docs/verification/windows-0.0.1.md).

## System requirements

- Windows 11, or a verified Windows 10 x64 build
- WebView2 runtime (the installer installs it)
- Speech recognition models, about 2.9 GB, download on first run; the storage location is changeable

## License and acknowledgments

This project is licensed under **AGPL-3.0-or-later**; see [LICENSE](LICENSE) for the full text. Third-party code, fonts, and models keep their own licenses; see [third-party rules](third_party/README.md).
