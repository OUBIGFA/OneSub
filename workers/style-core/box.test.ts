import { expect, test } from 'bun:test';
import { measureStyle } from './main';
import { prepareSubtitleFonts } from './smartsub/main/helpers/fontResolver';
import { buildAssDocument, wrapForBox, type FontMeasure } from './smartsub/main/helpers/assStyleBuilder';
import { DEFAULT_STYLE } from './smartsub/renderer/components/subtitleMerge/constants';

// Exact background boxes (OneSub patch). The pixel result is verified by
// burning with FFmpeg's libass; these tests pin the generated structure.

// Every character is one unit wide: wrapping depends on counts only.
const fixed: FontMeasure = {
  lineHeight: 24,
  ascent: 20,
  width: (text) => [...text].length * 10,
  ink: (text) => (text.trim() ? { top: 15, bottom: 3 } : null),
};
const box = { ...DEFAULT_STYLE, borderStyle: 3, outline: 2, alignment: 2, marginL: 20, marginR: 20, marginV: 20 };

test('wrapped lines are balanced and keep closing marks off line starts', () => {
  const text = '这是一条很长很长的字幕，用来检查自动换行之后每一行的背景框都与文字保持同样的边距';
  const lines = wrapForBox(text, fixed, 200);
  expect(lines.join('')).toBe(text);
  expect(lines.length).toBe(2);
  expect(Math.abs([...lines[0]].length - [...lines[1]].length)).toBeLessThanOrEqual(1);
  expect(lines.every((line) => !/^[，。、]/.test(line))).toBe(true);
  const english = 'Mixed English words stay whole when wrapped';
  expect(wrapForBox(english, fixed, 200).join(' ')).toBe(english);
});

test('each boxed line gets its own layer and band; text is drawn once above', () => {
  const cues = [{ startMs: 0, endMs: 1000, text: '第一行\n第二行' }];
  const ass = buildAssDocument(cues, box, undefined, () => fixed, 16 / 9);
  const events = ass.split('\n').filter((line) => line.startsWith('Dialogue:'));
  expect(events.length).toBe(3);
  // Lines stack upward from MarginV: line boxes 220–244 and 244–268,
  // baselines 240 and 264, ink 15 above and 3 below, padding 2.
  expect(events[0]).toContain('\\clip(m 0 223.00 l 384 223.00 384 245.00 0 245.00)');
  expect(events[1]).toContain('\\clip(m 0 247.00 l 384 247.00 384 269.00 0 269.00)');
  // Box layers keep the fill transparent; only their own line has a border.
  expect(events[0]).toContain('{\\q2\\1a&HFF&\\shad0\\clip(');
  expect(events[0]).toContain('{\\xbord1.5\\ybord2}第一行\\N{\\bord0}第二行');
  expect(events[1]).toContain('{\\bord0}第一行\\N{\\xbord1.5\\ybord2}第二行');
  expect(events[2]).toMatch(/^Dialogue: 2,.*\{\\q2\\bord0\\shad0\}第一行\\N第二行$/);
});

test('touching bands split at the middle so a translucent box is painted once', () => {
  // With padding 4 the bands 221–247 and 245–271 overlap; they meet at 246.
  const ass = buildAssDocument([{ startMs: 0, endMs: 1000, text: '第一行\n第二行' }], { ...box, outline: 4 }, undefined, () => fixed);
  expect(ass).toContain('\\clip(m 0 221.00 l 384 221.00 384 246.00 0 246.00)');
  expect(ass).toContain('\\clip(m 0 246.00 l 384 246.00 384 271.00 0 271.00)');
});

test('cues shown together keep SmartSub box, which libass can stack', () => {
  const cues = [
    { startMs: 0, endMs: 2000, text: '甲' },
    { startMs: 1000, endMs: 3000, text: '乙' },
    { startMs: 4000, endMs: 5000, text: '丙' },
  ];
  const ass = buildAssDocument(cues, box, undefined, () => fixed);
  expect(ass).toContain('Dialogue: 0,0:00:00.00,0:00:02.00,Default,,0,0,0,,甲');
  expect(ass).toContain('Dialogue: 0,0:00:01.00,0:00:03.00,Default,,0,0,0,,乙');
  expect(ass).toContain('\\clip(');
  expect(ass.match(/\\clip\(/g)?.length).toBe(1);
});

test('outline mode never gets a box layer', () => {
  const ass = buildAssDocument([{ startMs: 0, endMs: 1000, text: '字幕' }], { ...box, borderStyle: 1 }, undefined, () => fixed);
  expect(ass).not.toContain('\\clip(');
});

test('Microsoft YaHei is measured with the OS/2 line height libass uses', async () => {
  if (process.platform !== 'win32') return;
  await prepareSubtitleFonts();
  const measure = measureStyle({ ...box, fontName: 'Microsoft YaHei', fontSize: 24 })!;
  expect(measure.lineHeight).toBe(24);
  // winAscent 2167 of 2703 units; CJK ink spans 1723 above to 283 below.
  expect(measure.ascent).toBeCloseTo((24 * 2167) / 2703, 3);
  const ink = measure.ink('字幕测试中文')!;
  expect(ink.top).toBeCloseTo((24 * 1723) / 2703, 3);
  expect(ink.bottom).toBeCloseTo((24 * 283) / 2703, 3);
  expect(measure.ink('   ')).toBeNull();
});
