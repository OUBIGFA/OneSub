# Segmentation Reference

Use `repair → translate → divide`. Ordinary SRT/VTT may change boundaries; explicit
strict mode and ASS/SSA translation keep the existing timeline. A whole sentence is the
unit of understanding, not necessarily a single subtitle block.

## Find a defect before removing a seam

Read both sides of each boundary:

1. Does the first side finish a usable phrase?
2. Does the next side begin a usable phrase, rather than supplying a stranded word or
   argument of the previous side?

These are phrase tests, not a requirement that every subtitle be a full sentence. Keep
natural multi-block sentences. Topic continuity, fast speech, or a shared subject alone
is not a reason to chain-merge their blocks.

| Defect | Source example | Repair |
|---|---|---|
| Orphan completion | `...in the geometry` / `nodes.` | Reunite the term. |
| Stranded connector | `...based on the` / `same technology` | Keep connector and complement together. |
| Dependent opening | `add some` / `turbulence` | Translate the full verb/object phrase. |
| Split UI term | `Rope` / `Dynamics tag` | Preserve one searchable label. |
| Trailing modifier | `create variations` / `using MoGraph` | Translate together, then find a target-language seam. |
| Filler-only flash | `Right.` for 0.14 s | Remove only empty filler; combine with its own thought if needed. |

A repair must not add bridging words or repeat subjects to make the pieces look complete.
Each operation, qualifier, number and relation must remain traceable to the source.

## Example: repair without over-merging

Source:

```srt
4
00:00:03,985 --> 00:00:10,760
we are going to take a look at one of many methods on how to create variations of stuff

5
00:00:10,760 --> 00:00:12,445
using MoGraph.
```

`介绍一种创建随机变化的方法` / `使用 MoGraph` fails twice: the trailing modifier
is detached, and “random” was invented. Read the source as one thought, then use a natural
rendering such as `来看看用 MoGraph 做出各种变化的其中一种方法` within its combined
3.985–12.445 s span. Local Chinese grammar can put the means before the verb; this does
not authorize moving a separate downstream operation to an earlier time window.

If a shorter multi-piece rendering is needed, possible thought boundaries are
`来看看其中一种方法 ¦ 用 MoGraph 做出各种变化`. Determine the interior time from audio
or the source cue evidence, not from the `¦` marker or target character counts alone.
Without audio, report that a newly estimated interior time was not acoustically verified.
An intact phrase may retain a long-duration warning rather than receive an artificial cut.

## ASR repair and acoustic evidence

Repair clear mishearings, broken word boundaries and missing sentence punctuation.
Preserve the speaker's meaning and register; do not formalize spoken grammar or guess
an uncertain parameter/name. A hesitation or physiological breath does not prove that a
syntactic thought has ended. Conversely, adjacent timestamps do not prove uninterrupted
speech. Never claim to have heard a pause from subtitle timestamps alone.

**Keep the corresponding source time window.** Removing `okay, so` or shortening one
phrase does not make its time slot available for a later action. Do not pull a later
operation, screen change, or multi-clause explanation into an earlier short filler cue.
Check actual speech when audio is available; otherwise use the source cues conservatively.

## Gaps and coverage

- Preserve a source gap when both sides are usable phrases and the translation has a
  natural seam there.
- Cross a gap only for a specific seam defect. Record the affected source cues, gap and
  reason. A long gap deserves stronger evidence; it is not automatically forbidden.
- The checker reports every covered source gap beyond its 2 ms numerical tolerance,
  including gaps below `--pause-gap`. The configurable pause proxy groups spans for
  length-fidelity and nearby-boundary review; it does not authorize a merge or erase a gap.
- Output gaps must not uncover source speech. A preserved or shortened source gap is
  legal; a moved gap over source content is not. A cue entirely inside a source gap is
  invalid even if it merely touches the preceding/following cue edges.
- Overlapping ASS events are independent layers, not adjacent fragments to merge. Keep
  all events and their non-Text fields; compare their coverage as a union of intervals.

## Divide the translation

Cut at a natural phrase/clause boundary: between operations, before a connective that
belongs to the following clause, or between a topic and its comment. Avoid separating
names, technical terms, number/unit pairs, modifiers from their heads, or verbs from
necessary complements. Do not pad a piece just to make it a full sentence.

Evaluate duration and Display Units together using [reading-load.md](reading-load.md).
The configured thresholds prompt review; neither width nor a target piece count is a
cutting rule. Plan balanced pieces before assigning times, then verify each phrase fits
its corresponding spoken material. Splitting a dense sentence does not create more total
reading time.

## Re-read both directions

- Read A → B: does A leave a required complement hanging until B?
- Read B alone: is it a phrase, or only an orphan tail?
- Compare A+B with the source: did repair add padding, drop a condition/hedge, or move
  downstream content forward?

The checker's Chinese suffix/prefix word lists are only heuristics. `看`, `开始`, `地`,
and similar strings can occur in perfectly valid phrases or words. Do not replace a
natural phrase, invent a noun, or move a boundary merely to silence such a warning.

## Before delivery

Confirm that every removed boundary fixes a defect, every new boundary has a natural
seam and timing evidence, every crossed gap has a reason, and no source speech is left
uncovered. Run the checker against the source and review its warnings. It verifies file
structure and interval coverage, not audible pauses, word-level alignment, translation
quality, or grammatical completeness.
