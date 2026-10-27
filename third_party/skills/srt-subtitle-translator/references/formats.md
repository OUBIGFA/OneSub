# Formats: SRT, WebVTT, ASS/SSA

Keep the source format unless the user requests conversion. Preserve protected markers
and non-dialogue structure; the checker compares them when `--source` is supplied.

## SRT

- Write UTF-8 with sequential indices from 1 and blank lines between blocks.
- Keep timestamp spelling in strict mode. Ordinary output uses `HH:MM:SS,mmm`.
- Preserve inline formatting and positioning, including `<i>`, `<b>`, `<font>` and `{\an8}`.
- Rebuild indices after ordinary-mode merge/split operations. The assembler does this
  automatically even when each part starts numbering at 1.

Strict mode keeps block count, order, index, timestamp line, and each block's ordered
protected markers. A whole-file tag count alone is not sufficient: moving formatting to
another cue is a change even when no tag is lost.

## WebVTT

Keep the `WEBVTT` header/metadata, NOTE/STYLE/REGION blocks, cue settings, identifiers,
escapes, voice/class/ruby tags and karaoke timestamps. STYLE and REGION blocks belong
before the first cue; NOTE blocks may also appear between cues. A header must start with
the exact `WEBVTT` token, not a longer word, and cannot contain a timing arrow.

Ordinary-mode boundary changes:

- A merged cue keeps the first covered cue's ID and settings. Report differences in
  discarded settings rather than pretending every cue setting survived the merge.
- For a split, keep the source ID on the first piece only; later pieces omit it. All
  pieces retain the source settings. Do not create a duplicate ID just to satisfy mapping.
- Preserve the original marker inventory. Avoid splitting inside a styled or karaoke-timed
  run if it would require duplicating/reassigning tags; use a format-aware, explicitly
  reviewed treatment rather than silently flattening markup.

VTT timestamps use decimal milliseconds, optionally without hours. Preserve source
spelling in strict mode. Static blocks do not count as cues.

## ASS/SSA

Translate only the Text field of Dialogue events, in both ordinary and strict modes.
Use the Events `Format:` declaration: field names must be unique and Text must be last
because its content may contain commas. Preserve:

- sections, styles, field layout, and non-Dialogue records;
- event count and file order, even if starts are not chronologically sorted;
- every non-Text field, including Start, End, Layer, Style, Name, margins and Effect;
- override tags, karaoke timing, drawing/positioning codes, and `\N`, `\n`, `\h` markers.

Do not merge, split, retime, or discard a layered sign. Overlap is legal for layered ASS
and receives a review warning, not an error. Matching events by their position in the
file avoids confusing a sign with the dialogue underneath it. Resegmentation of styled
ASS is outside this translation workflow and needs separate authorization and validation.

## Markup and bilingual output

Keep paired formatting tags balanced and escapes escaped. Entities are decoded only for
load measurement, not rewritten in the file. Translate sound-cue words when appropriate
(`[music]` → `[音乐]`), preserving their brackets. Literal UI/code strings remain literal.

For bilingual output, put the translation first and source wording second. Do not
blindly duplicate protected tags on both lines: retain the markers with translated text,
or wrap the paired cue once when that preserves their scope. Review mixed style/voice
runs manually. `--max-lines 2` permits two lines; it does not prove that the source wording
was retained or that every markup scope is semantically correct.

The checker compares marker inventories for ordinary SRT/VTT and ordered markers per cue
in strict mode and ASS. It is not a renderer or a semantic markup-alignment checker.

## Explicit conversion

Explain format losses before conversion:

- VTT → SRT loses cue settings, NOTE/STYLE/REGION blocks, and may lose identifiers.
- ASS/SSA → SRT/VTT loses style, positioning, layers, drawing/karaoke information, and
  override effects; explicit breaks become ordinary line breaks needing review.

Cross-format `--source` comparison is not supported. Validate the converted file
independently, compare its text and timing manually, and report unverified structure.
Do not use `--format` to disguise a conversion; it only selects a parser.
