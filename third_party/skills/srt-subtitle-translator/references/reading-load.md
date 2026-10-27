# Reading Load

Use this reference for duration, CPS, scan-width, or short-duration-overload findings.
The executable thresholds live only in [language_profiles.json](../config/language_profiles.json).
These are house review thresholds, not universal readability limits or proof of compliance
with a streaming platform's current guide.

## Counting contract

The checker measures configured display-cost units (DU), not actual pixels:

- `cjk`: a full-width letter/character costs 1 DU; a narrow letter/digit costs 0.5.
  Punctuation, symbols, whitespace and control characters add no cost.
- `raw`: count displayed characters, including internal spaces; trim the outer spaces.
- Strip formatting tags before measurement and decode entities (`&amp;` displays as `&`).
- **English parenthetical content costs zero in all load statistics: DU, CPS and scan
  width.** This intentional exemption includes full-width or ASCII parentheses, nested
  English annotations, and a line containing only an annotation. English punctuation
  does not cancel the exemption. Keep Chinese parenthetical payload in the count.
- The exemption changes statistics only. It must not remove original names, values or
  annotations from the delivered subtitle, and it does not replace a visual preview.

For example, `基础形体（Blocking）` costs 4 DU and `脉冲修改器（Pulse Modifier）` costs
5 DU in CJK mode. Do not add an unrequested alternative counter that charges the English
annotation back to the budget.

## Duration and width are separate review signals

| Check | Shipped setting / meaning |
|---|---|
| Minimum duration | 0.833 s; shorter cues may flash by |
| Maximum duration | 7 s; longer cues need a reason, not an automatic cut |
| New split piece | Review below 1 s |
| Chinese reading speed | **4.5 DU/s** for `zh` and `zh-hant` |
| Chinese scan width | 25 DU per line |
| English reading speed / scan width | 20 characters/s / 42 per line |
| Other languages | Select their profile; do not apply the Chinese quota |

Do not insert a new inter-cue gap merely to imitate a platform frame-gap recommendation.
The source-coverage contract takes precedence unless the user authorizes retiming.

For planning, compare a cue's cost with both `duration × max_cps` and `max_width`.
There is no minimum three-character allowance and no need to round a quota up. At the
Chinese 4.5 DU/s setting, 2 seconds provides about 9 DU of review capacity, not 18–25 DU.
Aiming near 4 DU/s gives some margin, but do not pad short speech to fill that capacity.

A 25-DU line can fit the scan threshold yet be far too dense for 1.5 seconds. Conversely,
a short line may legitimately last 8 seconds because that is how long the thought takes.
Width thresholds are not guaranteed screen-width caps: fonts and players differ.

## Respond to a warning

1. Compare with the source for invented padding or downstream text placed too early.
2. Shorten wording without losing payload, uncertainty, negation or register.
3. Re-audit unnecessary merging. Split only at natural thought boundaries with timing
   evidence; dividing the same text over the same duration does not lower average CPS.
4. Preserve the line if a forced change would damage fidelity or violate strict/ASS
   timing. Explain the retained warning and the evidence limitation instead of claiming
   all checks passed. A user-specified hard platform limit may require a separate timing
   decision; do not fabricate compliance.

A **short-duration overload** is an additional high-priority warning when CPS already
exceeds the selected profile, duration is at most 2.5 s, and counted load is at least
10 units. Inspect it for unreadable density or content shifted forward. It is not proof
of desynchronization, nor permission to cut meaningful words.

## Other review signals

- **Over-merge:** fewer than 70% of the original blocks prompts a seam-by-seam audit.
  Every removed boundary needs a defect-based reason, regardless of the ratio.
- **Length fidelity:** with at least eight usable source spans, the checker compares each
  span's output/source text-volume ratio with the file's median. Outliers suggest padding
  or lost payload. A cross-gap cue is credited proportionally, not counted twice.

These are heuristics. A uniform omission can evade the median check; a legitimate term
or bilingual line can alter ratios. Below the sample minimum the checker makes no
length-fidelity judgement. Always compare meaning manually, and report the actual
`X errors | Y warnings` rather than equating exit code 0 with warning-free output.
