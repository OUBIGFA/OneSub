# Style Reference — Chinese target

Rules that apply when the target language is Chinese, on top of
`references/style-common.md`. Chinese subtitles are read faster than they are
"translated" in the viewer's head, so density and rhythm matter as much as accuracy —
and Chinese carries more meaning per character than English, so a faithful translation
is usually too long until condensed.

## Lean Chinese: the padding checklist

Use the same two-pass order for Chinese technical subtitles:先保留信息载荷，再删去无意义
填充词。ASD-STE100 的目标是可读和无歧义，不是把口语改成公文。删除前逐项确认条件、
否定、数值、因果、程度和不确定性仍在；删除后再检查每个字幕块是否只保留一个完整意群。

Chinese lets you write a grammatical, pleasant-sounding line that says noticeably more
than the speaker did, and the extra words are hard to notice because each one is idiomatic
on its own. This list is what to hunt for when re-reading a finished batch. Every item is
deletable without touching the meaning — if deleting it *does* change the meaning, the
word was payload and stays.

| Padding pattern | Example (padded → lean) |
|---|---|
| Subjects the source omitted | `我们把它往上挪一点` → `往上挪一点` |
| Stock connectives inserted between blocks | `接下来我们加个倒角` → `加个倒角` |
| Category nouns glued to verbs/adjectives (`效果`/`操作`/`方式`/`情况`/`部分`/`距离`/`数值`) | `添加一个倒角效果` → `加个倒角`；`移动一点距离` → `挪一点` |
| Invented necessity or permission (`需要`/`可以`/`应该`) | `我们需要打开面板` → `打开面板` |
| Closers the speaker never said (`就可以了`/`即可`/`了` piled on) | `设置为 20 就可以了` → `设成 20` |
| Formal verb compounds for plain actions (`进行`/`予以`+ verb) | `进行移动` → `挪`；`执行切割` → `切一刀` |
| Upgraded degree words | `a bit` → `很多`／`大幅` (wrong) → `一点` |
| Glossing a term the speaker just named | `倒角，也就是把边缘磨圆` when the audio said only `bevel` |
| Doubling a word with its own synonym | `修改调整一下` → `调一下` |

Two Chinese-specific temptations worth naming:

- **Four-character-idiom polish.** `效果非常自然流畅` when the speaker said `looks
  nice` — the idiom is good Chinese and a bad subtitle.
- **Restoring what spoken Chinese normally drops.** Written Chinese wants subjects,
  measure words and connectives that speech leaves out; a subtitle is speech. `把这个对象
  往上移动一点` is written; `往上挪一点` is what the line should be.

Cutting stops at the payload line: operations, values, names, numbers, negation, causal
links and hedges stay, even when the block is dense. Compress the wording, never the
content.

## Simultaneous clausal unfolding in Chinese (中文同声传译式展开语序)

Written Chinese prose strongly prefers fronting subordinate clauses: placing causal,
conditional, and concessive clauses before the main verb (`因为……所以……`, `虽然……但是……`).
However, subtitles represent spoken speech and are read concurrently with the audio.

- **Follow spoken unfolding over written grammar inversion**: when the speaker puts an
  afterthought, causal explanation, condition, or concession after the main action
  (`We rolled back because memory spiked`), use natural spoken Chinese with trailing
  connectives (`我们回滚了这次发布，因为内存激增了` or `我们回滚了发布，当时内存激增`).
- **Never invert clauses solely for literary elegance**: inverting a 2-clause sentence
  causes the viewer to read the reason on screen while hearing the action spoken,
  destroying visual-auditory alignment.
- Keep the clausal sequence matching the spoken timeline whenever spoken Chinese permits.

## Chinese-English typography

House style, not a platform requirement — Netflix's Simplified Chinese guide is silent on
CJK/Latin spacing, but the space makes mixed lines noticeably easier to scan:

- One half-width space between Chinese text and Latin words, numerals, software names,
  abbreviations, and English UI labels: `在 C4D 中设置 100%，亮度为 500nits`
- **中文词汇严禁前后留空格**：空格仅用于中英文/数字分界或意群间停顿，中文词汇之间绝不可因属于专有名词或 UI 术语而生硬留出前后空格（错误：`但 吸引 完全没有任何可视反馈`、`摩擦 还在 我先把 摩擦 关掉`）。此类词汇要么取消前后空格自然融入中文句式（`但吸引完全没有任何可视反馈`、`摩擦还在 我先把摩擦关掉`），要么直接保留英文原文并按中英混排规则留空格（建议保留英文原文：`但 Attractor 完全没有任何可视反馈`、`Friction 还在 我先把 Friction 关掉`）。
- No space between a number and its unit symbol, and no space around punctuation
- Use full-width Chinese punctuation inside the line (`，、`). Internal `？`
  is normal when a question is immediately followed by another clause within the same block.
  Colons `：` are strictly forbidden for introducing explanations, clauses, or reasoning
  in spoken dialogue — use a comma `，` or natural spoken phrasing instead. Reserve `：`
  exclusively for non-spoken structural markers: speaker labels (`John：`) or literal UI/code
  strings from the source. Exclamation marks (`！` and `!`) are strictly forbidden across
  all subtitles — never use exclamation marks in subtitle translation; express enthusiasm
  or urgency through natural vocabulary, or convert to plain declarative sentences.
  A translation-created `；` normally calls for a block split instead.
  Use half-width numerals (`1, 2, 3`, never `１２３`).

Wrong: `在C4D中设置100%,亮度为500nits`

## Single-thought blocks and segmentation marks

One Chinese subtitle block carries one complete thought. An internal full stop `。` or
semicolon `；` is a strong signal that the line contains two completed or separately
divided thought units. Split those units into separate blocks at a natural Chinese
boundary and distribute them across the speech span.

This is not a general punctuation ban. Commas `，`, enumeration commas `、`, and
question marks `？` are normal within one thought and do not justify a split by themselves.
(Exclamation marks `！`/`!` are forbidden throughout and must not appear in any block.)
Colons `：` are avoided in explanatory dialogue under this house style; use a comma or
natural spoken phrasing. Keep source speaker labels, literal UI/code strings, and menu
paths unchanged. The checker warns mechanically about internal `。`/`；` and exclamation
marks; it cannot distinguish every structural label from spoken wording.

Structural speaker labels, UI labels, menu paths, code, and strings the viewer sees typed
are exceptions and remain faithful to the source. A punctuation mark that belongs to
such a string is not a translation-created segmentation mark.

Examples:

- `首先选择对象；然后打开 Settings 面板` → split into `先选择对象` / `再打开 Settings 面板`
- `原因很简单：我们需要更多几何体` → **wrong**; ban colons in explanatory dialogue. Rewrite with a comma or natural connective: `原因很简单，我们需要更多几何体` or `因为我们需要更多几何体`
- `选择对象。然后打开面板` → split into two blocks rather than keeping an internal `。`
- `先选择对象，再打开面板` → keep the comma when it remains one thought (no exclamation mark)
- `位置、旋转和缩放都能调整` → keep the enumeration comma
- `John：请打开面板` → valid exception: keeps the structural speaker label

## Punctuation at line ends

Subtitles are timed, not typeset — the cut in time already ends the thought, so trailing
full stops, commas, or semicolons waste reading time and clutter the screen. Under the
Chinese profile (`final_punctuation: none`):
- **Never end a line with ordinary stopping punctuation:** Do not end a subtitle text line
  with `。．.` `;；` `:：` `，,` `、` `！!` or dashes. Trailing commas or pause marks (`，、`)
  are strictly forbidden at line ends — never leave a comma at the end of a block under
  the guise of "preserving punctuation".
- **Terminal question marks are explicitly exempted and preserved:** A genuine question
  at the end of a block must keep its full-width `？`. This mark conveys vital interrogative
  intonation and is not treated as a forbidden full stop.
- Exclamation marks (`！` and `!`) are completely banned from subtitles, both at line ends
  and within lines. Even if the speaker sounds excited or urgent, translate using natural
  spoken vocabulary without adding exclamation marks.

Punctuation inside a line is not a substitute for a subtitle boundary. Split when the
meaning contains two thought units, especially when `。` or `；` makes the separation
explicit. Do not split merely because a normal comma or enumeration comma appears internally.

A sentence that continues into the next block needs no decorative ellipsis, dash or
trailing comma. Use `…` (U+2026) only when it conveys a meaningful pause or interruption;
do not infer an audible pause from subtitle gaps alone.

No emoji, no citation markers, no translator notes, no bracketed commentary.

## Natural spoken phrasing

A subtitle line must be something a native speaker would say out loud — and only what this
speaker did say. Four failure patterns from real deliveries — check every finished batch
for them before writing:

- **Padding.** The line reads well and says more than the audio did. This is the most
  common defect of all, and the checklist at the top of this file is the fix.
- **Impossible verb–object pairings.** `搭建角色的大型` ("build the character's
  large-ness") — the noun is spoken shorthand that cannot take 搭建. Rewrite the idea
  with a real collocation: `搭建角色的基础形体`. If no natural collocation exists, the
  noun itself is insider slang and needs a plain-language replacement, not a better verb.
- **Garden-path segmentations.** Adjacent characters that invite a wrong parse:
  `多边形环绕过髋部` (多边形/环绕/过), `眼睛形状的观感受周围形状影响` (观感/受 vs
  观/感受), `在眼里得到两条环` (眼里 read as "in one's mind"). Rephrase so the intended
  parse is the only one: `这圈多边形绕过髋部`, `观感取决于周围形状`, `眼睛内部的两条环`.
- **Adjacent identical characters.** `取消勾选选中线框` jams on 取消勾选＋选中. Change
  one word or keep the UI label verbatim: `取消勾选 Selected Wireframe`.

- **短轴优先检查冗词，不删有效语气**：不要把口语扩写成书面长句。在短字幕中优先删除无意义
  填充和套话；“在某种程度上”“也许”等只要影响程度或确定性就必须保留。按
  [reading-load.md](reading-load.md) 结合时长检查，不再另设互相冲突的字数硬上限。

Also prefer the verbs practitioners actually speak — `切一刀`, `拉出一条边`, `把点拨一下`
— over bookish variants (`进行切割`, `执行边的拉出`). Reading each finished batch aloud
in your head is the cheapest full-coverage naturalness check there is: it catches both the
lines nobody would say and the lines that say too much.

### Streaming-platform variant

If the user wants Netflix-compliant subtitles, their Simplified Chinese guide differs from
the house style above: commas and full stops are dropped entirely and replaced by a single
space, `、` may join list items but not end a line, italics are forbidden, quotation marks
are full-width `“”`, and work titles take `《》`. Switch only on request, and say so.

## Names and terminology, Chinese specifics

- Read `references/names-zh.md` whenever Chinese subtitles contain personal, historical,
  literary, fictional, stage, or uncertain names. Its evidence order and consistency
  checks take precedence over ad-hoc transliteration.
- Use Chinese characters only for a name with an established Chinese rendering. When the
  current input already provides an English form and no established Chinese rendering
  is supported, preserve that English form exactly instead of creating a phonetic version.
- **首次出现括号中英双标**：重要行业概念首次出现时保留 `中文（English）`，如
  `基础形体（Blocking）`、`内部挤压（Inset）`；后续用短称。英文括号内容全部从 DU、CPS
  和扫描宽度负荷统计中扣除，不因长度或英文标点取消豁免；不要把统计豁免误当成删除注记。
- Pick the wording the target audience already uses, not the literal or dictionary one.
  A technically correct term the viewer has never seen costs them a re-read, and in a
  subtitle a re-read means the demonstration on screen is gone
- "The audience's wording" means wording that reads as language, not insider slang that
  only works spoken: 大型 for *blockout* is not a word to general viewers — 基础形体
  (with Blocking bracketed at first appearance) is. See the plain-language rule in
  `references/style-common.md`

- **LOCKED vs. preferred 术语分级执行**：用户明确点名的术语、代码标识符、命令行参数与模型名称（如 `--speculative-model`、`KV 缓存`）属于刚性锁定（LOCKED），必须逐字硬性执行、大小写与字符完全一致，严禁擅自意译、省略或增减空格；篇章提炼的行业概念属于柔性偏好（preferred），全文保持统一。
- **严禁输出斜杠备选译法**：每句字幕只提供一种最终定案译文。严禁在译文中使用斜杠提供备选词（如 `微调/蒸馏`、`提示词/指令`）或括号备选项。唯一允许出现斜杠的情况是原文标识符或命令本身自带斜杠（如 `/compact` 命令、URL 或分数值）。

### Domain glossaries

For 3D, motion-graphics, and VFX videos — Cinema 4D, Blender, Maya, 3ds Max, Houdini,
Redshift and friends — read `references/glossary-3d-zh.md` before translating.

Its core rule is worth stating here because it decides so many lines: **one term per
concept across every 3D application, using Cinema 4D's Simplified Chinese wording**, with
an exception for features that exist in one application alone. So `Inset` in a Blender
tutorial is 内部挤压, not Blender's own 内插面; `Extrude` in a Maya tutorial is 挤压, not
挤出; `Interpolation` is 插值, never 内插; `Viewport` is 视窗, never 视口.

**But it is a table of defaults, not a substitution list.** Context outranks it: the UI
wording when the viewer must find the thing on screen, spoken Chinese when the speaker is
narrating an action, and the right sub-field sense when one English word maps to several
Chinese ones. `Create a null` is 新建一个空对象 in speech even though the menu item reads
空白; a noisy render has 噪点 while a noise shader is 噪波. Mechanical substitution
produces the same unreadable subtitle as a bad translation — it just fails consistently.

For any other domain — cooking, finance, medical — no table ships with this skill. Build
the equivalent list during the first read, following the same two principles, and hold one
rendering per concept for the whole file unless the context genuinely changes the sense.
