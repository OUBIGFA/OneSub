import { expect, test } from 'bun:test';
import { validateCues, collectFonts, readControl } from './main';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import * as fontkit from 'fontkit';
import { prepareSubtitleFonts, loadFontData } from './smartsub/main/helpers/fontResolver';
import { buildAssDocument } from './smartsub/main/helpers/assStyleBuilder';
import { DEFAULT_STYLE, STYLE_PRESETS } from './smartsub/renderer/components/subtitleMerge/constants';

test('control pipe preserves Chinese paths and emoji across byte boundaries', async () => {
  const text = JSON.stringify({ requestId: '字幕🎬', inputPath: '作品/中文.srt' });
  const bytes = new TextEncoder().encode(text);
  async function* chunks() {
    for (let i = 0; i < bytes.length; i++) yield bytes.subarray(i, i + 1);
  }
  expect(await readControl(chunks())).toBe(text);
});

test('control pipe rejects oversized and incomplete UTF-8 input', async () => {
  async function* chunks(value: Uint8Array) { yield value; }
  await expect(readControl(chunks(new Uint8Array(1024 * 1024 + 1)))).rejects.toThrow('E_CONTROL_SIZE');
  await expect(readControl(chunks(new Uint8Array([0xe5, 0xad])))).rejects.toThrow();
});

test('SmartSub styles and position are executed without replacing its generator', () => {
  const cues = [{ startMs: 1000, endMs: 3000, text: '字幕' }];
  validateCues(cues);
  const ass = buildAssDocument(cues, DEFAULT_STYLE);
  expect(ass).toContain('PlayResX: 384');
  expect(ass).toContain('Dialogue: 0,0:00:01.00,0:00:03.00,Default,,0,0,0,,字幕');
  expect(STYLE_PRESETS.some(p => p.id === 'classic')).toBe(true);
  expect(STYLE_PRESETS.some(p => p.id === 'netflix_bilingual')).toBe(true);
});

test('bilingual order is independent of primary font and uses one event', () => {
  const cues = [{ startMs: 1000, endMs: 3000, text: 'main', secondaryText: '次字幕' }];
  const secondaryStyle = { ...DEFAULT_STYLE, fontSize: 16, italic: true, primaryColor: '#FFFF00' };
  for (const secondaryFirst of [true, false]) {
    const ass = buildAssDocument(cues, DEFAULT_STYLE, { secondaryStyle, secondaryFirst });
    expect(ass).toContain('Style: Secondary,');
    expect(ass).toContain(',16,&H0000FFFF,');
    expect(ass).toContain(secondaryFirst ? '{\\rSecondary}次字幕\\N{\\rDefault}main' : '{\\rDefault}main\\N{\\rSecondary}次字幕');
    expect(ass.match(/^Dialogue:/gm)?.length).toBe(1);
  }
});

test('OneSub rejects invalid cues instead of accepting upstream filtering', () => {
  expect(() => validateCues([{ startMs: 1, endMs: 2, text: 'too short after ASS quantization' }])).toThrow();
  expect(() => validateCues([{ startMs: 100, endMs: 200, text: '' }])).toThrow();
  expect(() => validateCues([{ startMs: -1, endMs: 200, text: 'invalid' }])).toThrow();
});

test('SmartSub regular/bold/italic variants reach both libass consumers', async () => {
  if (process.platform !== 'win32') return;
  await prepareSubtitleFonts();
  const directory = await mkdtemp(join(tmpdir(), 'onesub-fonts-'));
  try {
    const expected = loadFontData('Microsoft YaHei');
    expect(expected).not.toBeNull();
    const fonts = await collectFonts(['Microsoft YaHei'], directory);
    expect(fonts.length).toBe(1 + expected!.variants.length);
    const subfamilies: string[] = [];
    for (const font of fonts) {
      const data = await readFile(font.path);
      const parsed = fontkit.create(data);
      for (const face of parsed.fonts ?? [parsed]) subfamilies.push(face.subfamilyName);
    }
    expect(subfamilies.some(name => /bold/i.test(name))).toBe(true);
    // Repeated preview refreshes reuse verified content-addressed files.
    expect(await collectFonts(['Microsoft YaHei'], directory)).toEqual(fonts);
  } finally { await rm(directory, { recursive: true, force: true }); }
});
