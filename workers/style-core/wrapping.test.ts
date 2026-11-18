import { expect, test } from 'bun:test';
import { mkdtemp, rm } from 'node:fs/promises';
import { resolve } from 'node:path';
import { collectFonts, measureStyle } from './main';
import { buildAssDocument, type FontMeasure } from './smartsub/main/helpers/assStyleBuilder';
import { resolveAssFonts } from './smartsub/main/helpers/assFonts';
import { prepareSubtitleFonts } from './smartsub/main/helpers/fontResolver';
import { DEFAULT_STYLE } from './smartsub/renderer/components/subtitleMerge/constants';

const metric: FontMeasure = {
  lineHeight: 24,
  ascent: 20,
  width: (text) => [...text].length * 14,
  ink: (text) => text.trim() ? { top: 15, bottom: 3 } : null,
};
const text = '了解这些着色器的功能如何运作，会很有帮助，也能让你更轻松地组合不同的着色器来构建材质。';
const cue = { startMs: 120, endMs: 3200, text };
const events = (ass: string) => ass.split('\n').filter((line) => line.startsWith('Dialogue:'));
const visibleLines = (event: string) => event.split(',').slice(9).join(',').replace(/\{[^}]*\}/g, '').split('\\N');

test('long unspaced Chinese wraps inside the video margins without changing cue text or time', () => {
  let previousCount = 1;
  for (const aspect of [16 / 9, 9 / 16]) {
    const generated = events(buildAssDocument([cue], DEFAULT_STYLE, undefined, () => metric, aspect));
    expect(generated).toHaveLength(1);
    expect(generated[0]).toStartWith('Dialogue: 0,0:00:00.12,0:00:03.20,');
    expect(generated[0]).toContain('{\\q2}');
    const lines = visibleLines(generated[0]);
    expect(lines.length).toBeGreaterThan(previousCount);
    previousCount = lines.length;
    expect(lines.join('')).toBe(text);
    const available = (384 - DEFAULT_STYLE.marginL - DEFAULT_STYLE.marginR) * aspect / (384 / 288);
    for (const line of lines) {
      expect(metric.width(line)).toBeLessThanOrEqual(available);
      expect(line).not.toMatch(/^[，。、]/u);
    }
  }
});

test('generated breaks preserve both language styles, authored lines and matching glow lines', () => {
  const second = 'Short second language\nwith an authored break';
  const style = { ...DEFAULT_STYLE, glow: 1, secondLineColor: '#00FF00', highlightTerms: [text], highlightColor: '#FF0000' };
  const generated = events(buildAssDocument([{ ...cue, text: text + '\n手动换行', secondaryText: second }], style,
    { secondaryFirst: true, secondaryStyle: { ...DEFAULT_STYLE, fontSize: 12 } },
    (s) => s === style ? metric : { ...metric, width: (s) => metric.width(s) / 2 }, 4 / 3));
  expect(generated).toHaveLength(2);
  const lines = visibleLines(generated[1]);
  expect(lines).toEqual(visibleLines(generated[0]));
  expect(lines.slice(0, 2)).toEqual(second.split('\n'));
  expect(lines.slice(2, -1).length).toBeGreaterThan(1);
  expect(lines.slice(2, -1).join('')).toBe(text);
  expect(generated[1]).toEndWith('\\N{\\1c&H00FF00&\\1a&H00&}手动换行');
  expect(generated[1]).toContain('{\\rSecondary}');
  expect(generated[1]).toContain('{\\rDefault}{\\1c&H0000FF&\\1a&H00&}');
});

test('a token wider than the frame breaks only between graphemes', () => {
  const cluster = 'e\u0301';
  const word = cluster.repeat(25);
  const [event] = events(buildAssDocument([{ ...cue, text: word }], DEFAULT_STYLE, undefined, () => metric, 9 / 16));
  const lines = visibleLines(event);
  expect(lines.length).toBeGreaterThan(1);
  expect(lines.join('')).toBe(word);
  expect(lines.every((line) => /^(e\u0301)+$/u.test(line))).toBe(true);
});

const ffmpeg = process.env.ONESUB_FFMPEG;
test.skipIf(process.platform !== 'win32' || !ffmpeg)('libass burns long Chinese wholly inside landscape and portrait frames with font fallback', async () => {
  await prepareSubtitleFonts();
  const scratch = await mkdtemp(resolve(import.meta.dir, '../../_temp/subtitle-wrap-test-'));
  try {
    // Arial needs an explicit CJK fallback; measure the font used to render it.
    const style = { ...DEFAULT_STYLE, fontName: 'Arial', bold: true, fontSize: 24, shadow: 0 };
    for (const [width, height] of [[640, 360], [360, 640]]) {
      const ass = buildAssDocument([{ ...cue, startMs: 0 }], style, undefined, measureStyle, width / height);
      const resolved = resolveAssFonts(ass, style.fontName);
      expect(resolved.fontNames.length).toBeGreaterThan(1);
      await collectFonts(resolved.fontNames, resolve(scratch, 'fonts'));
      await Bun.write(resolve(scratch, 'render.ass'), resolved.content);
      const result = Bun.spawnSync([ffmpeg!, '-v', 'error', '-f', 'lavfi', '-i', `color=black:s=${width}x${height}:r=1`,
        '-vf', 'ass=render.ass:fontsdir=fonts', '-frames:v', '1', '-pix_fmt', 'gray', '-f', 'rawvideo', 'pipe:1'],
      { cwd: scratch, stdout: 'pipe', stderr: 'pipe' });
      expect(result.exitCode, result.stderr.toString()).toBe(0);
      expect(result.stdout.length).toBe(width * height);
      let minX = width;
      let maxX = -1;
      let bands = 0;
      let lastInk = -10;
      for (let y = 0; y < height; y++) {
        let ink = false;
        for (let x = 0; x < width; x++) {
          if (result.stdout[y * width + x] < 128) continue;
          minX = Math.min(minX, x);
          maxX = Math.max(maxX, x);
          ink = true;
        }
        if (ink) {
          if (y - lastInk > 4) bands++;
          lastInk = y;
        }
      }
      expect(bands).toBe(visibleLines(events(ass)[0]).length);
      expect(bands).toBeGreaterThan(1);
      expect(minX).toBeGreaterThanOrEqual(Math.floor(width * style.marginL / 384));
      expect(maxX).toBeLessThan(width - Math.floor(width * style.marginR / 384));
    }
  } finally {
    await rm(scratch, { recursive: true, force: true });
  }
}, 20000);
