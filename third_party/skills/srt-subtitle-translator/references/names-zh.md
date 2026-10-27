# Chinese Personal-Name Handling

Use this reference whenever Simplified or Traditional Chinese subtitles contain a
person's name, historical or literary name, fictional character, stage name, alias, or
an uncertain name form.

## Core rule

Choose exactly one of these outcomes for each person:

1. Keep an established Chinese rendering.
2. Keep the English or other source-language form already present in the current input.

Do not create a Chinese phonetic rendering merely because the name is unfamiliar. A
Chinese spelling produced by automatic translation or appearing in only one unverified
source does not establish a conventional rendering.

## Evidence order

Retain a Chinese rendering when it is supported by the strongest available evidence,
in this order:

1. A national, government, industry, or other formal naming standard
2. The person's, institution's, publisher's, or official body's own Chinese usage
3. A stable rendering used across multiple independent authoritative Chinese sources
4. A recognized standard Chinese edition for a historical, literary, or fictional work

If none of these supports the Chinese form, use the source-language form. “Commonly
seen once” is not the same as “established.”

## Preserve the source form

When the source form is used:

- Preserve spelling, capitalization, spaces, hyphens, apostrophes, diacritics, initials,
  and suffixes.
- Keep the level of detail supplied by the input. Do not expand a first name, surname,
  initial, or partial name into a full name.
- Keep full names, first names, and surnames in the forms in which they appear; do not
  mix a Chinese transliteration with an English initial or surname.
- Keep distinct people distinct even when their surnames or transliterations resemble
  each other.
- If the current input does not provide a reliable source-language spelling, report the
  name as unresolved instead of inventing a spelling.

Examples:

```text
Yvonne G.       → Yvonne G.
Ryan            → Ryan
Ryan A.         → Ryan A.
Vasco Vukov     → Vasco Vukov
Vasil Vukov     → Vasil Vukov
```

## Special categories

- Historical people, literary characters, fictional characters, stage names, and aliases
  follow the same evidence order as ordinary personal names.
- Preserve a recognized Chinese form for a work's standard edition when one exists.
- Do not translate a literary name into a descriptive Chinese nickname just to reproduce
  a pun or its surface meaning. If no stable Chinese form is available, keep the source
  wording and use it consistently, for example `Muster Mark`.
- Translate titles or honorifics separately from the name; they do not justify changing
  the name itself.

## Whole-subtitle consistency

Before editing, scan the complete subtitle and make a name table with the source form,
chosen rendering, full/short variants, evidence status, and any unresolved issue. Apply
one rendering per person throughout the subtitle. Use a short form only when the source
uses a short form or the established Chinese usage clearly supports it; never introduce
information that is absent from the input.

For Chinese typography, put one half-width space between Chinese text and Latin names.
Do not add spaces around Chinese punctuation. Run the subtitle checker after the name
pass and report retained Chinese forms, names returned to the source form, and unresolved
names separately.
