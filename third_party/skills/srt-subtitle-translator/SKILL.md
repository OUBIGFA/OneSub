---
name: srt-subtitle-translator
description: Use when translating SRT, VTT, ASS, or SSA subtitles, repairing subtitle segmentation during translation, or cleaning ASR captions while preserving timing and meaning.
metadata:
  short-description: Translate and repair timed subtitle files
---

# Subtitle Translator

Translate faithfully, remove only empty filler, and place boundaries where the target
language reads naturally. Use the user's target language; default to Simplified Chinese
only when none is given.

## Scope

Use for subtitle translation, requested bilingual output, or meaning-preserving ASR
cleanup. Do not start translating merely because a subtitle file is attached. Timing-only
audits, extraction, muxing/burning, and conversion without translation are separate tasks.
A transcript without timestamps keeps its paragraph structure; do not invent a timeline.
For cleanup-only requests, keep the source language and do not apply the default
translation target.

Translate with the current assistant. Local scripts inspect, assemble, and validate;
they do not translate. This workflow does not invoke translation APIs, browser translators,
MT plugins, or local MT engines. If the user explicitly requests such a provider, explain
this boundary and use a separate authorized workflow rather than silently substituting one.

## Core contract

- **Fidelity before brevity.** Preserve operations, values, names, conditions, sequence,
  causal links, negation, degree, uncertainty, warnings, and speaker intent. Remove
  filler only when it carries none of these. Never buy readability by deleting payload.
- Preserve spoken register. Add a subject or connective only when target-language
  grammar requires it, not to pad, formalize, or bridge a cut.
- Prefer source clausal unfolding order (同声传译式展开语序) among natural translations.
  Local grammatical reordering is allowed; do not front a later action or explanation
  merely to sound like written prose.
- **LOCKED**: user-mandated renderings and literal names, identifiers, commands, flags,
  paths, and on-screen strings. Preserve them exactly. Domain glossary entries are
  **preferred** defaults unless explicitly locked; select by sense and UI context.
- Deliver one final rendering, not slash-separated alternatives or variant notes.
  Slashes in source paths, URLs, commands, fractions, and other literal content stay.
- Correct only well-supported ASR mishearings and broken word/punctuation boundaries.
  Do not invent an uncertain name, number, or intended operation; report uncertainty.
- Keep the source format unless conversion is requested. Keep protected markup with
  the text it affects; do not duplicate, drop, or move it to unrelated cues.

## Timing and format modes

| Mode | Contract |
|---|---|
| Ordinary SRT/VTT | Repair a nameable seam defect, translate the whole thought, then divide naturally. Preserve source coverage and useful boundaries. |
| Strict SRT/VTT, only on request | Keep cue count, order, indices/IDs/settings, timestamp lines, and each cue's ordered markup. Use `--source` and `--strict`. |
| ASS/SSA, all modes | Change Dialogue Text only. Preserve event count/order, non-Text fields, layered signs, sections, and existing breaks. Do not merge, split, or retime events. |

Read [segmentation.md](references/segmentation.md) for boundary decisions, and
[formats.md](references/formats.md) for VTT, ASS/SSA, markup, or conversion.

- Merge only a specific defect: orphan tail, stranded connector, split term, dependent
  opening, trailing modifier, or filler-only flash block. Topic continuity is not enough.
- A subtitle gap is not proof of audible silence. Cross one only to fix a nameable defect;
  report each crossing. Do not create a gap that leaves source speech uncovered.
- Keep translated content within its corresponding source thought's time window. Removing
  filler does not free its timestamps for a later operation. With audio, anchor cuts to
  speech; without it, use source cues and disclose estimated interior cuts.
- Preserve existing speaker lines and ASS breaks. Use one line for ordinary dialogue;
  bilingual or multi-speaker material may require `--max-lines 2` or the existing line count.

## Style and reading load

Read [style-common.md](references/style-common.md) for translation and
[style-zh.md](references/style-zh.md) for Chinese. For technical/instructional speech,
use [asd-ste100-subtitles.md](references/asd-ste100-subtitles.md): first preserve the
information payload (保留信息载荷), then remove zero-information filler (删去无意义填充词).
This is controlled-expression guidance, not a claim of ASD-STE100 compliance.

Build a working glossary from the whole file. For Chinese names, read
[names-zh.md](references/names-zh.md). For Simplified Chinese 3D/motion/VFX tutorials,
read [glossary-3d-zh.md](references/glossary-3d-zh.md), preserving its context-sensitive
C4D-aligned defaults. Use `译名（original）` at an important concept's first appearance,
then the short form; do not annotate every ordinary word.

**English parenthetical content is fully exempt from DU, CPS, and scan-width load
statistics.** This is intentional house policy. Keep the annotation in the subtitle;
subtract it only from load measurements, including annotation-only and nested cases.
Do not remove it merely to reduce a reported reading budget.

[config/language_profiles.json](config/language_profiles.json) is the numeric source of
truth. The Chinese reading-speed threshold remains **4.5 DU/s**. Read
[reading-load.md](references/reading-load.md) for duration/width warnings and
[language-profiles.md](references/language-profiles.md) for custom or regional profiles.
Do not invent a second hard quota or delete meaning to satisfy a warning.

## Workflow

1. Read the complete source: domain, register, recurring terms, UI strings, probable ASR
   errors, and existing structure. Load only references relevant to this task.
2. Audit seams; repair, translate, then divide within the selected mode. Re-read each
   batch for missing payload, padding, natural phrasing, and terminology consistency.
3. Write UTF-8 beside the source using `source-zh.srt`, `source-zh-hant.srt`, `source-en.srt`,
   or analogous language/format suffixes; use `-bi` for bilingual output. Never overwrite
   the input. If the output exists, choose a new name unless replacement is authorized.
4. For a translation built in parts, use `scripts/assemble_subtitle.py`. It rebuilds SRT
   indices, validates parts, preserves one matching VTT/ASS header, and refuses silent
   metadata loss. It does not accept ad-hoc `start|end|text` rows. Resolve script paths
   relative to this skill directory, not the subtitle directory.
5. Run the checker on the complete output, not just individual batches:

   ```text
   python <skill-dir>/scripts/check_subtitle.py <output> --source <input> --lang <target>
   ```

   Add `--strict` only for an untouched timeline, `--max-lines 2` for two-line output,
   and `--lang-config` only for a supplied project profile. `--json` returns structured
   findings. `--quiet` hides warning details, so do not use it as the review evidence.
6. Fix errors. Investigate reading load, short-duration overload, broken seams, heavy
   merging, padding/dropped-payload, and markup warnings. Correct avoidable defects;
   explain retained warnings when fidelity or an untouched timeline prevents a fix.
7. Check beginning/middle/end (or every cue in a very short file), every moved boundary,
   and dense short cues against the source. Check audio only when supplied and accessible;
   otherwise explicitly say that acoustic alignment was not verified. The checker cannot
   establish translation fidelity, correct terminology, or audio synchronization.
8. Clean this task's intermediates and report any leftovers before finishing.

## Workspace and cleanup

Put intermediates beside the source in `.srt-subtitle-translator-work`, using a unique
suffixed sibling if that folder already exists. Do not reuse unknown files. Keep source
and final output outside it. With no source path, use a unique workspace `_temp/` folder.
For safe platform cleanup and part-file commands, read [edge-cases.md](references/edge-cases.md).
Keep parts while fixing validation failures; when the task ends, clean this task's files
whether validation succeeded or failed. Never claim cleanup succeeded without checking.

## Delivery

Reply briefly with the output path, mode, block count before/after, exact checker counts
(`X errors | Y warnings`), warning categories/reasons, each crossed gap and its repair
reason, unresolved ASR/timing issues, and cleanup status. Zero errors does not mean zero
warnings or verified audio. Do not put explanations, citations, or translator notes in
subtitle text.
