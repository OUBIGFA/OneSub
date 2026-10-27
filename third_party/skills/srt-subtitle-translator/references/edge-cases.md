# File Handling and Delivery

## Output and parts

Write UTF-8 beside the source and retain its extension. Use a target-language suffix
(`-zh`, `-zh-hant`, `-en`, etc.), or `-bi` for requested bilingual output. Deliver a file
rather than a long subtitle dump in chat. Do not overwrite the input or an existing
output without explicit authorization.

For parts, resolve the script path from the skill directory:

```text
python <skill-dir>/scripts/assemble_subtitle.py <output.srt> <work/part1.srt> <work/part2.srt>
```

- SRT parts are valid SRT; each may start numbering at 1. Assembly renumbers the result
  without changing dialogue, timing or markup. Ad-hoc pipe-separated rows are unsupported.
- The first VTT part contains its document header. Later parts may be cue/body blocks or
  complete VTT with the same header. All body blocks are kept; invalid STYLE/REGION
  placement is rejected rather than silently moved or dropped.
- The first ASS/SSA part is a complete document. Later parts are event lines, or complete
  documents with matching metadata and Format. Later events are inserted inside Events,
  before trailing sections such as Fonts. Conflicting or unrecognized content fails.
- Empty/malformed parts, duplicated input paths, format mismatches, and output paths
  aliasing an input are rejected. Existing outputs require `--overwrite`; use it only
  when replacement was authorized.

Validate the complete assembled output with `check_subtitle.py --source ... --lang ...`.
Assembly validation does not replace source comparison or translation review.

## Workspace and cleanup

Create `.srt-subtitle-translator-work` beside the source; if it already exists, create a
unique suffixed sibling instead of adopting or emptying it. Put all task-created parts,
logs, data, copied/generated helpers and validation output there. Keep the source and
final deliverable outside. Without a source path, use a unique workspace `_temp/` folder.

Keep parts while correcting validation failures. When ending the task, recycle this
task's intermediates even if validation failed; preserve the source and final output.
Check that cleanup actually happened and report exact remaining paths if it did not.
Do not remove pre-existing files or follow links outside the owned task folder.

On Windows, use an available recycle capability. For a known task-owned directory,
PowerShell can use the standard recycle-bin API:

```powershell
Add-Type -AssemblyName Microsoft.VisualBasic
[Microsoft.VisualBasic.FileIO.FileSystem]::DeleteDirectory(
  '<absolute-task-owned-directory>', 'OnlyErrorDialogs', 'SendToRecycleBin')
Test-Path -LiteralPath '<absolute-task-owned-directory>'
```

For one file, use `DeleteFile` instead. On other platforms, use an available trash/recycle
operation. If the environment blocks the operation or has no suitable capability, report
the leftovers; do not bypass the restriction through another API or claim success.
Permanent deletion is not the fallback unless the user authorizes it.

## Encoding

The checker accepts UTF-8, UTF-8 BOM, GB18030 and UTF-16 sources. Outputs and assembly
parts must be UTF-8. A legacy console code page does not establish file corruption;
inspect the file itself. Report actual mojibake or uncertain decoding rather than silently
inventing repaired text.

## Anomalies

| Case | Response |
|---|---|
| Reversed/zero duration or malformed timestamps | Report the source defect; do not invent replacement times or claim validation succeeded. |
| Source overlap | Preserve and flag unless repair is authorized; ASS layers may legitimately overlap. |
| Source gap | Preserve unless crossing repairs a nameable defect. It is not proof of audible silence. |
| Dense short cue | Check padding and time-window mapping first; preserve all meaningful payload. |
| Existing speaker lines or ASS breaks | Keep them, raising `--max-lines` to the existing count when needed. |
| Bilingual output | Translation above source wording; review both lines and markup scope. Two-line permission is not bilingual-content validation. |
| No audio | Compare source text/timestamps; explicitly leave acoustic alignment unverified. |

Do not turn a validation failure into an endless retry loop: correct recoverable defects;
if a source anomaly or missing evidence prevents validation, say what remains unresolved.
The delivery reply includes exact error/warning counts and cleanup status, not notes inside
subtitle text.
