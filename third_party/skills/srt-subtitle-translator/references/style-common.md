# Style Reference — every target language

Everything here serves one goal: a viewer reads the subtitle once, at speed, and gets the
meaning the speaker actually expressed, without re-reading. These rules are
language-independent. On top of them, apply the target language's own conventions — for
Chinese, `references/style-zh.md`; for other targets, that language's standard subtitle
practice (English keeps normal sentence punctuation, for example).

## Fidelity first, then economy

For technical and instructional subtitles, apply the ASD-STE100-inspired reference
`references/asd-ste100-subtitles.md`. It adds a controlled-expression check to this
section: record the information payload first, then delete only words that add no
meaning. It does not license summaries, invented subjects, or formal wording that
changes the speaker's register.

Two operations look similar and are opposites. Getting them straight is most of what
separates a professional subtitle from a plausible one.

- **Removing noise** — filler, hesitation, abandoned starts, or repetition only when
  those tokens add no information, emphasis, uncertainty, or speaker intent.
- **Adding smoothness** — invented framing, explanations, or repeated subjects added
  just to connect two blocks. Supply only what target-language grammar needs to express
  the source meaning; do not enlarge the claim.

So the working rule is: **the shortest wording that still says everything the source
said.** Not shorter — a summary is not a subtitle. Not longer — a subtitle is read in a
glance while the viewer is also watching the screen.

Each of these padded lines reads pleasantly and is still a fidelity failure:

| Source | Padded (wrong) | Lean (right) | What went wrong |
|---|---|---|---|
| `Move it up a bit` | `我们把它稍微向上移动一点距离` | `往上挪一点` | `我们`, `距离` and the formal verb are all invented |
| `That's it` | `以上就是本次教程的全部内容了` | `就这样` | a whole sentence built from two words |
| `Now add a bevel` | `接下来我们需要为它添加一个倒角效果` | `加个倒角` | `需要`, `效果` and the framing are not in the audio |
| `It looks better` | `这样看起来会好很多，效果更自然` | `这样好看些` | second clause invented; `很多` overstates |
| `Set it to 20` | `把这个数值设置为 20 就可以了` | `设成 20` | `就可以了` adds a judgement the speaker didn't make |

**Padding patterns to hunt for, in any target language:**

- Subjects the source left out (`我们`/`你`)
- Stock connectives inserted between blocks to make them flow (`接下来`/`那么`/`然后`)
- Category nouns glued onto verbs and adjectives (`效果`/`操作`/`方式`/`情况`/`部分`/`距离`)
- Politeness, necessity and permission the speaker never used
  (`我们需要`/`可以尝试`/`就可以了`)
- Degree words upgraded — `a bit` is not `很多`
- Glossing: explaining a term the speaker had just named
- Formal verb compounds standing in for plain actions (`进行移动` for `挪`)

Three habits that keep a file lean:

- **Trace every word back to the audio.** If a word in your line answers to nothing in
  the source, it is padding — delete it and re-read the line. It will almost always still
  be complete.
- **Let length track the source.** A two-word remark becomes a two-word subtitle. When
  your line runs much longer than the speech it covers, suspect padding before you suspect
  the timing. Much shorter, and suspect that something was summarized away.
- **Keep the speaker's register.** Casual speech stays casual; a formal lecture stays
  formal. Do not promote a plain-spoken tutorial into written prose, and do not flatten a
  precise explanation into breeziness.

**What may never be cut**, however tight the window: operations, parameter values, object
and menu names, numbers and units, visual judgements, warnings, causal links (`because…`,
`otherwise…`), negation, and hedges that change how certain the claim is (`probably`,
`about`, `I think`, `roughly`). Those are the payload — if a block is still over-dense
after honest de-padding, split it or accept the density. Never buy reading comfort with
meaning.

## Simultaneous interpreting clause order (同声传译式展开语序)

Viewers read subtitles in real time while hearing the speaker talk: each line is narrow,
stays on screen for only two to three seconds, and disappears as the next line appears.
Subtitles are timed broadcast captions, not written essays.

The most common way to ruin a subtitle is translating it "like good written prose":
inverting clauses, pulling trailing conditions or reasons to the front, or rearranging
thoughts to make an elegant written sentence. Such translations read well in isolation
as text, but on screen they completely desynchronize eye and ear — the viewer hears
clause A, but reads the translation of clause B.

**The simultaneous interpreting rule:** Among equally natural renderings in the target
language, **always choose the one whose clausal unfolding sequence follows the source
audio.** Only invert clauses when target-language grammar strictly and genuinely requires
it.

| Source | Inverted prose (wrong) | Simultaneous order (right) | Why |
|---|---|---|---|
| `So what we ended up doing, and this took us a while to figure out, was to let the model review first.` | `我们花了好一阵子才想明白，最后决定让模型先审一遍。` | `我们最后的做法，花了好一阵子才想明白，就是让模型先审一遍。` | The wrong line moves the second half to the front: when hearing "what we ended up doing", the viewer sees "花了好一阵子", breaking temporal synchrony. |
| `We rolled back the deployment because the memory usage was spiking.` | `因为内存占用激增，我们回滚了部署。` | `我们回滚了部署，因为内存占用激增了。` | Spoken conversation naturally uses trailing causal clauses; inverting it forces the viewer to wait for the action. |
| `I decided to join the team, although it took five months to convince me.` | `虽然花了五个月才说服我，但我还是决定加入团队。` | `我决定加入团队，虽然这花了好五个月才说服我。` | Follows the speaker's emotional and narrative progression. |

When dividing a sentence, compare each piece with the corresponding source cues. This
helps preserve unfolding order but does not prove word-level audio alignment.

## Noise filtering

Spoken audio is full of tokens that carry no information. Removing them is not
liberty-taking — it is what makes the line readable in its time window.

Remove when used as filler, hesitation, or self-agreement:

- English sources: `yeah`, `okay`, `alright`, `nice`, `cool`, `uh`, `um`, `like`,
  `you know`, `sort of`, `I mean`, `right?` as a tag question
- Chinese sources: 嗯、哦、呃、这个、那个、就是说、实际上
- Every language has its own set — identify it during the first full read

Keep the same words when they carry meaning:

- `Click OK` → the OK is a UI button, keep it
- `keep it cool` → temperature, not filler
- `Is that right?` asked as a real question → keep the question

Condense fragmented self-talk instead of transcribing it:

- `Cool, let's check this, yeah, maybe this is okay` → "let's check this — maybe it's okay"
- `I mean, like, maybe just move this over here` → "maybe move this over here"

Keep `maybe` when it expresses uncertainty or a tentative suggestion. Condensing does
not turn a possibility into a confident claim, nor expand it into extra evaluation.

Never remove: operations, parameter values, object or menu names, visual judgements,
warnings, causal explanations ("because…", "otherwise…"), negation, and hedges that change
how certain the claim is. Those are the payload.

## Symbols and numbers

Symbols read faster than spelled-out words and save horizontal space, in every language.

| Spoken | Write |
|---|---|
| negative 50 | -50 |
| 360 degrees | 360° |
| 20 percent | 20% |
| 10 by 10 | 10×10 |
| from 10 to 20 | 10~20 |
| greater than 5 / less than 3 | >5 / <3 |
| plus or minus 5 | ±5 |

Use Arabic numerals for settings, counts, frames, steps, versions, and any value the user
types into a field. Spell out numbers only where the target language idiom demands it
(`第一步`, "first of all").

## Units

Use standard symbols, attached to the number with no space: `50kg`, `100m`, `220V`,
`50Hz`, `500nits`, `3cm`, `24fps`. Do not expand them into words in the target language —
the expansion reads slower and takes more space.

## Names and terminology

- Personal names use an established target-language rendering only when it is supported
  by an official naming standard, an official/institutional usage, or stable conventional
  usage in authoritative sources; otherwise preserve the source form. For non-Latin
  source names, use an established official romanization when one exists rather than
  inventing a target-language transliteration.
- Software, renderers, plugins, algorithms, file formats, and acronyms stay in their
  original form unless a stable target-language term exists and is more familiar
- On first appearance of an important technical concept, keep `译名（original）` so the
  viewer can identify it in the UI or literature; use the short form thereafter. English
  parenthetical content is fully exempt from DU, CPS, and scan-width statistics under
  the house policy in [reading-load.md](reading-load.md). This is a counting exemption,
  not permission to delete annotations or add variant translations.
- Menu paths, buttons, and parameter names are what the viewer must find on screen. If
  the software they are watching ships an English UI, keep the label in English. If a
  localized UI is standard for that tool in the target market, use the localized label.
- **Plain language outranks insider shorthand.** A community coinage that a general
  viewer cannot parse from its characters is a defect even if experts say it aloud:
  Chinese modeling slang renders *blockout* as 大型, which reads as "large-scale" to
  everyone else — describe the concept instead (基础形体, base form) and bracket the
  original term at first appearance. The "audience's word" rule below applies to words
  the audience reads as words, not to slang that only works spoken.
- **Names the viewer types stay verbatim.** Object names, file names, layer names, and
  anything being entered on screen are copied, not translated — the subtitle must match
  the string the viewer sees appearing in the UI.

**Build a glossary before translating.** Scan the whole file first, list every recurring
term and UI label, decide one rendering for each, and hold it for the entire file. An
inconsistent glossary is the most common defect in long tutorial subtitles — the same
button called three different things in twenty minutes.

**Where a glossary already exists for the domain and target, start from it.** Do not
re-derive a term you could look up: for 3D and motion-graphics videos into Chinese, that
list is `references/glossary-3d-zh.md`. Deriving a term yourself is a last resort, not a
starting point — a literal translation that is technically defensible but unfamiliar to
the audience is still a defect.

**A glossary is a table of defaults, not a substitution list.** Applying one mechanically
produces exactly the stilted, obviously-translated subtitle this skill exists to prevent.
Context outranks the table in three recurring situations:

- **On-screen label vs. spoken narration.** When the viewer must find the thing in a menu
  or a parameter field, use the UI wording — matching the string on screen is the point.
  When the speaker is just describing what they are doing, use how a native creator would
  say it out loud. The same term can take both forms in one file.
- **One source word, several target words.** Technical vocabulary is full of words whose
  correct rendering depends on the sub-field — a "noisy render" and a "noise shader" are
  not the same word in Chinese. Getting this wrong doesn't read as awkward; it reads as
  nonsense.
- **A sense the glossary didn't anticipate.** When the video uses a term outside the sense
  the list was built for, translate the sense, not the row.

Consistency means one rendering per *concept*, not one rendering per *string*.

Two traps when deriving a term yourself:

- **The audience's word beats the application's word.** If viewers of every tool in a
  field learned the vocabulary from one dominant application, use that application's
  terms even when translating a competitor's tutorial. Consistency across the field is
  worth more than fidelity to one product's own localization.
- **The mainland-Simplified word beats the Traditional Chinese one.** They diverge on
  common technical vocabulary, and the Traditional form reads as foreign or simply
  unparseable to a Simplified Chinese audience.

### Terminology tiers: LOCKED vs. preferred (术语两级锁定机制)

Divide terms into two distinct discipline levels:

- **LOCKED (刚性锁定)**: User/project-mandated renderings, literal CLI commands and
  flags, product/model names, code identifiers, URLs, and strings visibly typed on screen.
  Preserve spelling, case and internal spacing exactly. Do not treat every entry in the
  bundled domain glossary as locked.
- **preferred (柔性偏好)**: Bundled domain defaults and terminology derived from the file.
  Maintain one rendering per concept, allowing the actual UI label, target locale and
  contextual sense to decide when the default does not fit.

### Single final rendering mandate (no slash alternatives)

Subtitles are finished broadcast captions, never drafts or translators' working notes:

- **Every sentence must give exactly ONE definitive, screen-ready rendering.**
- **Never emit slash-separated alternatives** (such as `微调/蒸馏` or `选项A / 选项B`).
  Pick the single most faithful, natural term and commit to it.
- **Never emit bracketed variant explanations** (such as `提示词（或称指令）`).
- Keep literal slashes in source identifiers, paths, URLs, commands, fractions or units.
  The prohibition concerns translator alternatives, not slash characters themselves.

## ASR error repair

Auto-captions mishear words that the context makes obvious. Repair the meaning rather
than translating the mistake:

- `road track` in a rope-simulation tutorial → the speaker said `Rope tag`
- `Powerbill` → `power bill`
- Numbers misheard as words, or a product name mangled into common words
- Domain terms mangled into common words — when a word makes no sense in context, ask
  which domain term it *sounds like*: `a chromium` → acromion (肩峰), `squat` → quad
  (四边形), `font tag` → Phong tag, `apparent` → parent, `the jar` → the jaw, `Guru
  shading` → Gouraud shading, `cilia` → silhouette, `Slide bends` → slight bends,
  `F2, N3 and N4` → possibly F2, F3 and F4. Context supplies candidates, not proof:
  verify uncertain names/numbers with available evidence and report unresolved readings.

Use surrounding cues, domain context and available audio. Repair a likely reading only
when sufficiently supported. Otherwise translate the recoverable meaning, keep uncertainty
visible in the delivery report, and do not invent the missing operation, value or name.

Repairs stay inside the block they belong to; do not move content across boundaries to
"fix" the transcript (when blocks are merged, repairs go into the merged block).

### Boundaries of ASR repair (ASR修复的分工边界)

Keep the boundary between correcting mishearings and unauthorized timeline rewriting crystal clear:

- **What ASR repair does**:
  - Correct acoustic mishearings, garbled proper nouns, and phonetically butchered domain terms.
  - Fix split words that the recognizer falsely separated into fragments.
  - Restore punctuation boundaries (sentence-ending full stops, commas) so long streams
    of speech do not run on endlessly with commas.
- **What ASR repair never does**:
  - **Never rewrite spoken grammar**: do not formalize conversational speech into bookish
    prose, and do not adjust subject-verb syntax to make the speaker look more eloquent.
  - **Never delete spoken payload**: pauses, stumbles, and hesitations are cleaned up as
    filler only when they carry zero information; never delete words that express operations,
    conditions, or hedges on the timeline.
  - **Never cut on dangling connectors**: never place a subtitle cut after a dangling clausal
    connector or preposition (`because.`, `of.`, `since.`, `以及。`, `当。`). Attach it to the
    continuation clause.
  - **Never treat physiological pauses as syntactic boundaries**: speakers pause to breathe,
    think, or drink water. An acoustic gap is evidence of silence, not evidence that a thought
    has closed.
