# ASD-STE100-Inspired Controlled Subtitle Expression

ASD-STE100 is a controlled-English standard for technical documentation. This
reference adapts its useful goals to timed subtitles: clear wording, one meaning per
term, direct sentences, and no avoidable words. It is a translation aid, not a literal
approved-word list. Target-language grammar, the speaker's register, and the source
meaning remain authoritative.

## Apply the two-pass contract

1. **Information pass.** List the source payload before shortening: action, object,
   value and unit, condition, sequence, cause, negation, warning, uncertainty, degree,
   name, UI label, and speaker intent. Each item must appear in the translation or be
   explicitly marked unresolved in the work notes; never put a translator note inside
   the subtitle.
2. **Economy pass.** Remove a word only when its removal keeps every payload item,
   relationship, and level of certainty. Delete filler, hesitation, self-repair,
   duplicated meaning, invented subjects, stock transitions, and empty closers. Do not
   shorten by summarizing.
3. **Clarity pass.** Prefer a concrete verb and a short active clause. Use one
   rendering per concept. Replace an ambiguous pronoun or vague noun only when the
   source context identifies the referent; never invent a subject or object to make a
   line sound complete.
4. **Subtitle pass.** Keep one complete thought per block, split at a natural target
   language seam, and confirm that the line can be read at the configured speed.

## Controlled-expression rules

- Prefer common, concrete verbs: `set`, `open`, `move`, and `check` are clearer than
  nominalized or formal verb phrases. In Chinese, prefer `设`, `打开`, `挪`, and `检查`
  over `进行设置`, `予以打开`, or `执行检查`.
- Keep the shortest natural clause that carries the source. Do not add `we`, `you`,
  `need`, `can`, `should`, `effect`, `operation`, `distance`, or a closing phrase
  unless the source expresses that meaning.
- Use one term for one concept across the file. Keep UI labels, names, file names,
  values, and searchable on-screen text unchanged unless a target-language UI form is
  required by the project's glossary.
- Preserve small words that control truth: `if`, `only`, `still`, `roughly`, `not`,
  `unless`, and comparative or degree terms. A shorter line that drops one of these
  fails the information pass.
- Remove `um`, `uh`, `like`, `okay`, repeated starts, and abandoned repairs only when
  they add no confirmation, contrast, uncertainty, or action. Keep a meaningful `OK`,
  a real question, a speaker label, or a UI word.
- Keep the speaker's register. Controlled wording removes noise; it does not turn
  casual speech into formal prose or add an explanation for a term already spoken.
- Do not stack synonyms, adjectives, or four-character polish around a short source.
  Do not expand `looks fine` into `效果看起来已经相当不错了`.
- For English output, use the ASD-STE100 principles directly where they improve
  clarity. For other targets, apply the same goals through that language's natural
  grammar and subtitle conventions; do not force English syntax or vocabulary.

## Fast decisions

| Candidate text | Keep when | Remove or shorten when |
|---|---|---|
| hesitation or repeated start | it changes intent or marks an audible interruption | it only delays the payload |
| subject or connective | the source states it or the target grammar needs it | it was invented to smooth adjacent blocks |
| hedge or degree word | it changes certainty or amount | it merely repeats another modifier |
| UI/name/value | the viewer must find or type it | it is an unspoken explanation added by the translator |
| final phrase | it carries a result, limit, or warning | it is `that's it`, `you know`, or a polite closure with no payload |

## Final review questions

- Can every translated noun, verb, number, relation, and hedge be traced to the source?
- Did any deleted word carry a condition, negation, degree, cause, or warning?
- Can one word be removed without changing the claim? If yes, remove it.
- Does each block read as a complete target-language phrase without stranded grammar?
- Did the pass preserve the file format, protected text, register, and glossary?

If a source phrase is unclear, keep a readable translation of the available meaning
and report the uncertainty. Do not fill the gap with a guess, explanation, or translator
note inside the subtitle.
