/**
 * ASS 文档生成模块：SubtitleStyle → 完整 ASS 文本。
 *
 * 硬字幕烧录与预览（JASSUB）共用同一份生成逻辑，保证所见即所得。
 *
 * 脚本空间刻意沿用 PlayResX=384 / PlayResY=288 —— 这正是 ffmpeg 对 SRT
 * 隐式转 ASS 时使用的脚本空间（libass 默认），因此既有用户保存的字号、
 * 边距、描边、阴影数值的烧录观感与旧版 force_style 方案完全一致（零回归），
 * 同时语义从「隐式默认」变为「显式声明」：任意分辨率按 视频高度/288 等比缩放。
 */

import type {
  SubtitleStyle,
  SubtitleAlignment,
} from '../../types/subtitleMerge';
import type { SubtitleCue } from './subtitleFormats';
import { formatAssTime } from './subtitleFormats';
import { subtitleTextRuns, subtitleGlow } from '../../types/subtitleAppearance';
import { parseSubtitleColor } from '../../types/subtitleColor';
import { mapAssOverrideBlocks } from '../../types/assOverrides';
import {
  ASS_PLAY_RES_X,
  ASS_PLAY_RES_Y,
  absoluteSubtitleY,
  subtitleAnchor,
} from '../../types/subtitleCanvas';

/** 烧录/预览共用的 ASS 脚本空间（与 ffmpeg 的 SRT 隐式转换一致） */
export { ASS_PLAY_RES_X, ASS_PLAY_RES_Y } from '../../types/subtitleCanvas';

/** 背景不透明度缺省值（百分比，≈旧版硬编码 alpha=128） */
export const DEFAULT_BACK_OPACITY = 50;

/**
 * 将前端 numpad 风格的 Alignment 转换为 ASS/SSA legacy Alignment。
 *
 * 前端 numpad 风格：7/8/9=上排，4/5/6=中排，1/2/3=下排（左/中/右）。
 * SSA legacy 编码（仅用于 FFmpeg force_style 的兼容接口，不用于 V4+ Style 行）：
 *   底部 1/2/3；中部 9/10/11；顶部 5/6/7。
 */
export function convertAlignment(numpadAlignment: SubtitleAlignment): number {
  const alignmentMap: Record<SubtitleAlignment, number> = {
    1: 1,
    2: 2,
    3: 3,
    4: 9,
    5: 10,
    6: 11,
    7: 5,
    8: 6,
    9: 7,
  };
  return alignmentMap[numpadAlignment] || 2;
}

/**
 * 将 CSS 颜色转换为 ASS 颜色格式
 * CSS: #RRGGBB 或 rgba(r, g, b, a)
 * ASS: &HAABBGGRR（Alpha, Blue, Green, Red；alpha 00=不透明 FF=全透明）
 */
export function cssColorToAss(cssColor: string, alpha: number = 0): string {
  const color = parseSubtitleColor(cssColor);
  if (!color) throw new Error(`Invalid subtitle color: ${cssColor}`);
  // Color opacity and the separate background/glow opacity multiply.
  const opacity = color.opacity * (1 - Math.max(0, Math.min(255, alpha)) / 255);
  const clampedAlpha = Math.round((1 - opacity) * 255);
  const toHex = (v: number) => v.toString(16).padStart(2, '0').toUpperCase();

  return `&H${toHex(clampedAlpha)}${toHex(color.blue)}${toHex(color.green)}${toHex(color.red)}`;
}

export function assPrimaryColorTags(color: string): string {
  const ass = cssColorToAss(color);
  // Override colors use BGR; unlike style colors, their alpha is a separate tag.
  return `{\\1c&H${ass.slice(4)}&\\1a&H${ass.slice(2, 4)}&}`;
}

/** 背景不透明度（0-100%）→ ASS alpha（0-255，语义反转：00=不透明） */
export function backOpacityToAssAlpha(backOpacity: number | undefined): number {
  const opacity = Number.isFinite(backOpacity)
    ? Math.max(0, Math.min(100, backOpacity as number))
    : DEFAULT_BACK_OPACITY;
  return Math.round((1 - opacity / 100) * 255);
}

/**
 * 生成 [V4+ Styles] 的 Style 行。
 *
 * 颜色映射按 libass 实际取色语义（背景色 bug 的修复核心）：
 * - BorderStyle=3（背景框）：背景框由 OutlineColour 绘制、阴影区由 BackColour 绘制，
 *   两者均取用户背景色 + 用户不透明度（阴影同色，避免 shadow>0 时露出异色边）；
 *   Outline 数值即背景框 padding。该模式 libass 不绘制文字描边，描边色字段被占用无感知损失。
 * - BorderStyle=1（边框+阴影）：OutlineColour 取描边色（不透明），
 *   BackColour 取背景/阴影色 + 用户不透明度。
 */
export function buildAssStyleLine(style: SubtitleStyle): string {
  const assAlpha = backOpacityToAssAlpha(style.backOpacity);
  const isBoxMode = style.borderStyle === 3;

  const primaryColour = cssColorToAss(style.primaryColor);
  const outlineColour = isBoxMode
    ? cssColorToAss(style.backColor, assAlpha)
    : cssColorToAss(style.outlineColor);
  const backColour = cssColorToAss(style.backColor, assAlpha);

  // libass 仅在 border > 0 时绘制背景框（框 = 描边区域画成实心矩形）。
  // 背景框模式下 Outline 语义是框的 padding，钳到最小 1，
  // 避免「选了背景框但边框宽度为 0 → 完全看不到框」的困惑。
  const effectiveOutline = isBoxMode
    ? Math.max(style.outline, 1)
    : style.outline;
  // 背景框模式下 Shadow 会画出一个同色偏移的「影子框」（双重边缘观感），
  // UI 在该模式下不提供阴影设置，生成端同步钳 0，保证所配即所得。
  const effectiveShadow = isBoxMode ? 0 : style.shadow;

  // Style 行是逗号分隔的定长 CSV，字体名含逗号/换行会让后续字段整体错位、
  // libass 静默错渲。ASS 无转义语法，只能剥离（字体族名本身不含这些字符）。
  const safeFontName = style.fontName.replace(/[,\r\n]/g, ' ').trim();

  const fields = [
    'Default', // Name
    safeFontName, // Fontname
    String(style.fontSize), // Fontsize
    primaryColour, // PrimaryColour
    '&H000000FF', // SecondaryColour（卡拉OK用，不涉及）
    outlineColour, // OutlineColour
    backColour, // BackColour
    style.bold ? '-1' : '0', // Bold
    style.italic ? '-1' : '0', // Italic
    style.underline ? '-1' : '0', // Underline
    '0', // StrikeOut
    '100', // ScaleX
    '100', // ScaleY
    '0', // Spacing
    '0', // Angle
    String(style.borderStyle), // BorderStyle
    String(effectiveOutline), // Outline
    String(effectiveShadow), // Shadow
    String(style.alignment), // V4+ uses numpad alignment directly.
    String(style.marginL), // MarginL
    String(style.marginR), // MarginR
    String(style.marginV), // MarginV
    '1', // Encoding
  ];

  return `Style: ${fields.join(',')}`;
}

/** ASS Dialogue 文本转义：换行转 \N，剥离可能干扰解析的花括号覆盖标签起始符 */
function escapeAssText(text: string): string {
  return text
    .replace(/\r\n?/g, '\n')
    .replace(/\{/g, '｛') // 全角替换，避免被解析为覆盖标签
    .replace(/\}/g, '｝')
    .replace(/\n/g, '\\N');
}

export function styledAssText(text: string, style: SubtitleStyle): string {
  if (
    !style.secondLineColor &&
    !style.highlightTerms?.some((term) => term.trim())
  )
    return escapeAssText(text);
  return subtitleTextRuns(text, style)
    .map((run) => `${assPrimaryColorTags(run.color)}${escapeAssText(run.text)}`)
    .join('');
}

export function assGlowTags(style: SubtitleStyle): string {
  const glow = subtitleGlow(style);
  const color = cssColorToAss(style.glowColor || '#FFFFFF', 96);
  return glow
    ? `{\\1a&HFF&\\3c&H${color.slice(4)}&\\3a&H${color.slice(2, 4)}&\\bord${style.outline + glow}\\blur${glow}\\shad0}`
    : '';
}

export function assGlowText(text: string, style: SubtitleStyle): string {
  const tags = assGlowTags(style);
  return (
    tags +
    mapAssOverrideBlocks(text, (tag) => {
      if (tag.startsWith('\\r')) return `${tag}${tags.slice(1, -1)}`;
      if (/^\\1a(?:&|$)/.test(tag)) return '\\1a&HFF&';
      if (/^\\alpha(?:&|$)/.test(tag)) return `${tag}\\1a&HFF&`;
      return tag;
    })
  );
}

/**
 * OneSub patch: font measurements of one style at its ASS size, in script
 * units, for a background box that is exactly the requested padding away
 * from the glyphs. libass sizes a font so that OS/2 winAscent + winDescent
 * equals Fontsize, and its BorderStyle=3 box spans that whole line height
 * whatever the glyphs' ink; \ybord cannot go below zero.
 */
export interface FontMeasure {
  /** Line height libass gives the font: Fontsize. */
  lineHeight: number;
  /** Baseline below the top of the line box. */
  ascent: number;
  /** Advance width of a text run, for line breaking. */
  width(text: string): number;
  /** Ink above (top) and below (bottom) the baseline; null when blank. */
  ink(text: string): { top: number; bottom: number } | null;
}
export type MeasureStyle = (style: SubtitleStyle) => FontMeasure | null;

type BilingualOptions = { secondaryStyle: SubtitleStyle; secondaryFirst: boolean };

// Closing marks never start a line and opening marks never end one, as in
// CJK line-breaking rules (kinsoku).
const NO_LINE_START = new Set([...'，。、；：？！）》」』】〕〉’”…—·%),.;:?!]}']);
const NO_LINE_END = new Set([...'（《「『【〔〈‘“([{']);
const isWide = (character: string) =>
  /[\u2e80-\u9fff\uac00-\ud7af\uf900-\ufaff\uff00-\uffef\u3000-\u303f]/u.test(character);

/** Units between which a line may break: words with their spaces, single CJK characters. */
function breakUnits(text: string): string[] {
  const units: string[] = [];
  let word = '';
  const flush = () => {
    if (word) units.push(word);
    word = '';
  };
  for (const character of text) {
    if (NO_LINE_START.has(character) && (word || units.length)) {
      if (word) word += character;
      else units[units.length - 1] += character;
      continue;
    }
    if (isWide(character)) {
      flush();
      const last = units[units.length - 1];
      if (last && NO_LINE_END.has([...last].pop()!)) units[units.length - 1] += character;
      else units.push(character);
      continue;
    }
    if (word.endsWith(' ') && character !== ' ') flush();
    word += character;
  }
  flush();
  return units;
}

/**
 * Breaks one line into as few lines as fit, then balances them with the
 * upper lines widest, as libass's smart wrapping (WrapStyle 0) does. The
 * lines are written with explicit \N and \q2, so preview and burn-in use
 * exactly these lines.
 */
export function wrapForBox(text: string, measure: FontMeasure, maxWidth: number): string[] {
  if (measure.width(text) <= maxWidth) return [text];
  const units = breakUnits(text);
  const greedy = (limit: number) => {
    const lines: string[] = [];
    let current = '';
    for (const unit of units) {
      if (current && measure.width((current + unit).trimEnd()) > limit) {
        lines.push(current.trimEnd());
        current = unit.trimStart();
      } else current += unit;
    }
    if (current.trim()) lines.push(current.trimEnd());
    return lines;
  };
  const count = greedy(maxWidth).length;
  let low = measure.width(text) / count;
  let high = maxWidth;
  for (let step = 0; step < 24; step++) {
    const middle = (low + high) / 2;
    if (greedy(middle).length <= count) high = middle;
    else low = middle;
  }
  return greedy(high);
}

/** Cues shown at the same time as another; libass may move them on collision. */
function overlappingCues<T extends SubtitleCue>(cues: T[]): Set<T> {
  const sorted = [...cues].sort((a, b) => a.startMs - b.startMs);
  const overlapping = new Set<T>();
  let longest: T | undefined;
  for (const cue of sorted) {
    if (longest && cue.startMs < longest.endMs) {
      overlapping.add(cue);
      overlapping.add(longest);
    }
    if (!longest || cue.endMs > longest.endMs) longest = cue;
  }
  return overlapping;
}

/**
 * Exact background box for one cue: a layer under the text repeats it
 * invisibly with libass's own BorderStyle=3 box, clipped per line to the
 * glyphs' ink plus the padding. Left and right stay libass's (advance ±
 * padding); top and bottom become the ink ± padding instead of the font's
 * line height. Returns null when a style cannot be measured, which keeps
 * SmartSub's box.
 */
function exactBoxEvents(
  cue: SubtitleCue & { secondaryText?: string },
  timing: string,
  position: string,
  style: SubtitleStyle,
  bilingual: BilingualOptions | undefined,
  measure: MeasureStyle,
  horizontal: number,
): string[] | null {
  type Part = { name: string; style: SubtitleStyle; text: string };
  const primary: Part = { name: 'Default', style, text: cue.text };
  let parts: Part[] = [primary];
  if (bilingual && cue.secondaryText !== undefined) {
    const secondary: Part = { name: 'Secondary', style: bilingual.secondaryStyle, text: cue.secondaryText };
    parts = bilingual.secondaryFirst ? [secondary, primary] : [primary, secondary];
  }
  const maxWidth = ASS_PLAY_RES_X - style.marginL - style.marginR;
  type Line = { text: string; style: SubtitleStyle; measure: FontMeasure };
  const lines: Line[] = [];
  const texts: string[] = [];
  for (const part of parts) {
    const metrics = measure(part.style);
    if (!metrics) return null;
    const wrapped = part.text
      .replace(/\r\n?/g, '\n')
      .split('\n')
      .flatMap((line) => wrapForBox(line, metrics, maxWidth));
    for (const line of wrapped) lines.push({ text: line, style: part.style, measure: metrics });
    const styled = styledAssText(wrapped.join('\n'), part.style);
    texts.push(bilingual ? `{\\r${part.name}}${styled}` : styled);
  }
  const text = texts.join('\\N');
  // Vertical layout as libass does it: lines stacked by their line height,
  // the block placed by its alignment row against MarginV or the \pos anchor.
  const height = lines.reduce((sum, line) => sum + line.measure.lineHeight, 0);
  const anchor = subtitleAnchor(style);
  const row = Math.floor((style.alignment - 1) / 3);
  let top: number;
  if (absoluteSubtitleY(style) !== undefined) top = anchor.y - height * anchor.vertical;
  else if (row === 0) top = ASS_PLAY_RES_Y - style.marginV - height;
  else if (row === 1) top = (ASS_PLAY_RES_Y - height) / 2;
  else top = style.marginV;
  // Each boxed line gets its own band: the ink plus the padding. Where the
  // bands of neighbouring lines meet, they split at the middle, so a
  // translucent box is never painted twice.
  const bands: { line: number; y1: number; y2: number }[] = [];
  lines.forEach((line, index) => {
    const ink = line.measure.ink(line.text);
    if (line.style.borderStyle === 3 && ink) {
      const baseline = top + line.measure.ascent;
      const padding = Math.max(0, line.style.outline);
      const band = { line: index, y1: baseline - ink.top - padding, y2: baseline + ink.bottom + padding };
      const previous = bands[bands.length - 1];
      if (previous && previous.line === index - 1 && previous.y2 > band.y1) {
        previous.y2 = band.y1 = (previous.y2 + band.y1) / 2;
      }
      bands.push(band);
    }
    top += line.measure.lineHeight;
  });
  if (!bands.length) return null;
  // Box layers repeat the text invisibly; only one line keeps libass's box
  // in each, inside that line's band. A tiny border keeps libass drawing the
  // box at zero padding (1 px). libass scales \xbord with the frame width
  // over PlayResX but glyphs with the height, so the horizontal padding is
  // corrected by `horizontal`.
  const styleOf = (name: string) => (name === 'Secondary' && bilingual ? bilingual.secondaryStyle : style);
  const retag = (source: string, tags: (s: SubtitleStyle) => string) =>
    mapAssOverrideBlocks(source, (tag) => (tag.startsWith('\\r') ? tag + tags(styleOf(tag.slice(2).trim())) : tag));
  const hidden = (s: SubtitleStyle) => (s.borderStyle === 3 ? '\\1a&HFF&\\shad0' : '\\alpha&HFF&\\bord0\\shad0');
  // Colour runs (second-line colour, highlights) set their own fill alpha;
  // on box layers the fill stays transparent.
  const segments = mapAssOverrideBlocks(retag(text, hidden), (tag) => (/^\\1a(?:&|$)/.test(tag) ? '\\1a&HFF&' : tag)).split('\\N');
  // Per-line tags go after a line's leading style reset, which would clear them.
  const lineTags = (segment: string, tags: string) => {
    const leading = /^(?:\{[^}]*\})*/.exec(segment)![0];
    return `${leading}{${tags}}${segment.slice(leading.length)}`;
  };
  const border = (value: number) => Math.max(Number(value.toFixed(3)), 0.01);
  const events = bands.map((band, layer) => {
    const boxed = segments
      .map((segment, index) => {
        const s = lines[index].style;
        return lineTags(segment, index === band.line ? `\\xbord${border(s.outline * horizontal)}\\ybord${border(s.outline)}` : '\\bord0');
      })
      .join('\\N');
    const clip = `m 0 ${band.y1.toFixed(2)} l ${ASS_PLAY_RES_X} ${band.y1.toFixed(2)} ${ASS_PLAY_RES_X} ${band.y2.toFixed(2)} 0 ${band.y2.toFixed(2)}`;
    // Separate layers: libass moves colliding events only within one layer.
    return `Dialogue: ${layer},${timing}${position}{\\q2${hidden(style)}\\clip(${clip})}${boxed}`;
  });
  const textTags = (s: SubtitleStyle) => (s.borderStyle === 3 ? '\\bord0\\shad0' : '');
  const shown = `{\\q2${textTags(style)}}${retag(text, textTags)}`;
  const glow = assGlowTags(style);
  if (glow) events.push(`Dialogue: ${events.length},${timing}${position}{\\q2}${assGlowText(text, style)}`);
  events.push(`Dialogue: ${events.length},${timing}${position}${shown}`);
  return events;
}

/**
 * 生成完整 ASS 文档（Script Info + Styles + Events）。
 * 烧录与预览共用此函数，保证两端渲染输入一致。
 */
export function buildAssDocument(
  cues: (SubtitleCue & { secondaryText?: string })[],
  style: SubtitleStyle,
  bilingual?: BilingualOptions,
  measure?: MeasureStyle,
  frameAspect?: number,
): string {
  const anchor = subtitleAnchor(style);
  const position =
    absoluteSubtitleY(style) === undefined
      ? ''
      : `{\\an${style.alignment}\\pos(${Number(anchor.x.toFixed(3))},${Number(anchor.y.toFixed(3))})}`;
  const header = `[Script Info]
ScriptType: v4.00+
Collisions: Normal
PlayResX: ${ASS_PLAY_RES_X}
PlayResY: ${ASS_PLAY_RES_Y}
WrapStyle: 0
ScaledBorderAndShadow: yes

[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
${buildAssStyleLine(style)}
${bilingual ? buildAssStyleLine(bilingual.secondaryStyle).replace('Style: Default,', 'Style: Secondary,') + '\n' : ''}
[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
`;

  const shown = cues.filter((cue) => cue.text.trim() !== '' && cue.endMs > cue.startMs);
  const overlapping = overlappingCues(shown);
  const anyBox = style.borderStyle === 3 || bilingual?.secondaryStyle.borderStyle === 3;
  // Script units per pixel differ across and down when the video's aspect is
  // not PlayRes's 4:3; equal padding needs the ratio (1 when unknown).
  const horizontal = frameAspect && frameAspect > 0 ? ASS_PLAY_RES_X / ASS_PLAY_RES_Y / frameAspect : 1;
  const events = shown
    .flatMap((cue) => {
      const timing = `${formatAssTime(cue.startMs)},${formatAssTime(cue.endMs)},Default,,0,0,0,,`;
      // OneSub patch: exact box padding where the layout is known. Cues shown
      // together may be moved by libass's collision handling and keep
      // SmartSub's box.
      if (measure && anyBox && !overlapping.has(cue)) {
        const exact = exactBoxEvents(cue, timing, position, style, bilingual, measure, horizontal);
        if (exact) return exact;
      }
      let text = styledAssText(cue.text, style);
      // One bilingual event keeps both languages together during collision
      // layout. Named style resets provide independent typography without a
      // second ASS generator; block alignment/margins come from the main style.
      if (bilingual && cue.secondaryText !== undefined) {
        const primary = `{\\rDefault}${text}`;
        const secondary = `{\\rSecondary}${styledAssText(cue.secondaryText, bilingual.secondaryStyle)}`;
        text = bilingual.secondaryFirst ? `${secondary}\\N${primary}` : `${primary}\\N${secondary}`;
      }
      const glow = assGlowTags(style);
      return glow
        ? [
            `Dialogue: 0,${timing}${position}${assGlowText(text, style)}`,
            `Dialogue: 1,${timing}${position}${text}`,
          ]
        : [`Dialogue: 0,${timing}${position}${text}`];
    })
    .join('\n');

  return header + events + '\n';
}
