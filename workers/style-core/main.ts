import { createHash } from 'node:crypto';
import { readFile, open, mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import * as fontkit from 'fontkit';
import { buildAssDocument, type FontMeasure } from './smartsub/main/helpers/assStyleBuilder';
import { resolveAssFonts } from './smartsub/main/helpers/assFonts';
import { listSubtitleFonts, prepareSubtitleFonts, newFontFallbackReport, summarizeFontFallbackReport, resolveBurnFontName, loadFontData } from './smartsub/main/helpers/fontResolver';
import { DEFAULT_STYLE, STYLE_PRESETS } from './smartsub/renderer/components/subtitleMerge/constants';
import type { SubtitleStyle } from './smartsub/types/subtitleMerge';
import type { SubtitleCue } from './smartsub/main/helpers/subtitleFormats';

// The process adapter owns I/O and validation only. Font selection, presets and
// ASS construction execute SmartSub's modules (font-alias patch is registered).
// Its own errors are stable E_ codes that the desktop says in its language.
export function validateCues(cues: SubtitleCue[]) {
  if (!cues.length) throw new Error('E_NO_CUES');
  for (const cue of cues) {
    if (!cue.text.trim() || !Number.isSafeInteger(cue.startMs) || !Number.isSafeInteger(cue.endMs) || cue.startMs < 0 || cue.endMs <= cue.startMs || Math.round(cue.endMs / 10) <= Math.round(cue.startMs / 10)) {
      throw new Error('E_INVALID_CUE');
    }
  }
}

type Face = {
  postscriptName: string;
  familyName: string;
  unitsPerEm: number;
  italicAngle: number;
  'OS/2'?: { winAscent?: number; winDescent?: number; usWinAscent?: number; usWinDescent?: number; usWeightClass?: number };
  hhea: { ascent: number; descent: number };
  layout(text: string): { advanceWidth: number; glyphs: { bbox: { minY: number; maxY: number } }[] };
};

// Measures a style with the same font bytes SmartSub hands to libass. libass
// maps OS/2 winAscent + winDescent to Fontsize (VSFilter's rule), so one font
// unit is Fontsize / (winAscent + winDescent) script units. Bold uses the
// family's bold face when it has one, as the font provider selects it.
// Faces are parsed once per worker run; a document measures many cues.
const measuredFaces = new Map<string, Face | null>();
function styleFace(style: SubtitleStyle): Face | null {
  const key = JSON.stringify([style.fontName, !!style.bold, !!style.italic]);
  if (!measuredFaces.has(key)) {
    const loaded = loadFontData(style.fontName);
    let face: Face | null = null;
    if (loaded) {
      const faces: Face[] = [loaded.data, ...loaded.variants].flatMap((data) => {
        const font = fontkit.create(data) as unknown as Face & { fonts?: Face[] };
        return font.fonts ?? [font];
      });
      const regular = faces.find((f) => f.postscriptName === loaded.postscriptName) ?? faces[0] ?? null;
      const weight = (f: Face) => f['OS/2']?.usWeightClass ?? 400;
      face = (regular && style.bold && faces.find((f) => f.familyName === regular.familyName && weight(f) >= 600 && !!f.italicAngle === !!style.italic)) || regular;
    }
    measuredFaces.set(key, face);
  }
  return measuredFaces.get(key)!;
}

export function measureStyle(style: SubtitleStyle): FontMeasure | null {
  const face = styleFace(style);
  if (!face) return null;
  const os2 = face['OS/2'];
  const ascent = os2?.winAscent ?? os2?.usWinAscent ?? face.hhea.ascent;
  const descent = os2?.winDescent ?? os2?.usWinDescent ?? -face.hhea.descent;
  if (!(ascent + descent > 0)) return null;
  const scale = style.fontSize / (ascent + descent);
  return {
    lineHeight: style.fontSize,
    ascent: ascent * scale,
    width: (text) => face.layout(text).advanceWidth * scale,
    ink: (text) => {
      let top = -Infinity;
      let bottom = -Infinity;
      for (const glyph of face.layout(text).glyphs) {
        if (glyph.bbox.maxY <= glyph.bbox.minY) continue;
        top = Math.max(top, glyph.bbox.maxY);
        bottom = Math.max(bottom, -glyph.bbox.minY);
      }
      return top === -Infinity ? null : { top: top * scale, bottom: bottom * scale };
    },
  };
}

// Cache only SmartSub's exact selected family/variants, never redistribute a
// system font. Both libass consumers receive these same content-addressed bytes.
export async function collectFonts(names: string[], directory: string) {
  await mkdir(directory, { recursive: true });
  const fonts: { name: string; path: string; sha256: string }[] = [];
  const seen = new Set<string>();
  for (const name of names) {
    const loaded = loadFontData(name);
    if (!loaded) throw new Error(`E_FONT_UNAVAILABLE:${name}`);
    for (const data of [loaded.data, ...loaded.variants]) {
      const sha256 = createHash('sha256').update(data).digest('hex');
      if (seen.has(sha256)) continue;
      seen.add(sha256);
      // Fontkit/libass detect the format from bytes, including TTC collections.
      const path = join(directory, sha256 + '.ttf');
      try {
        const file = await open(path, 'wx');
        try { await file.writeFile(data); await file.sync(); } finally { await file.close(); }
      } catch (error) {
        if ((error as NodeJS.ErrnoException).code !== 'EEXIST') throw error;
        if (createHash('sha256').update(await readFile(path)).digest('hex') !== sha256) {
          throw new Error('E_FONT_CACHE');
        }
      }
      fonts.push({ name, path, sha256 });
    }
  }
  return fonts;
}

if (import.meta.main) {
  let requestId = '';
  try {
    let control = '';
    for await (const chunk of Bun.stdin.stream()) {
      control += new TextDecoder().decode(chunk);
      if (Buffer.byteLength(control) > 1024 * 1024) throw new Error('E_CONTROL_SIZE');
    }
    const request = JSON.parse(control);
    requestId = request.requestId;
    if (request.protocol !== 1 || !request.outputPath) throw new Error('E_PROTOCOL');
    await prepareSubtitleFonts();
    let result: unknown;
    if (request.operation === 'catalog') {
      const fontName = process.platform === 'win32' ? 'Microsoft YaHei' : process.platform === 'darwin' ? 'PingFang SC' : 'Noto Sans CJK SC';
      result = { presets: STYLE_PRESETS, defaultStyle: { ...DEFAULT_STYLE, fontName }, fonts: await listSubtitleFonts() };
    } else if (request.operation === 'build' || request.operation === 'resolve') {
      const raw = await readFile(request.inputPath);
      if (raw.byteLength > 64 * 1024 * 1024) throw new Error('E_INPUT_SIZE');
      const input = JSON.parse(raw.toString('utf8')) as { cues: (SubtitleCue & { secondaryText?: string })[]; ass?: string; style: SubtitleStyle; frameAspect?: number; bilingual?: { secondaryStyle: SubtitleStyle; secondaryFirst: boolean } };
      if (request.operation === 'build') validateCues(input.cues);
      else if (!input.ass?.trim()) throw new Error('E_NO_ASS');
      const style = { ...input.style };
      // Resolve with the exact same installed-font policy as SmartSub; report the
      // resolved name so missing fonts are not silently presented as the requested one.
      style.fontName = resolveBurnFontName(style.fontName, /[\u3400-\u9fff]/u.test(input.ass ?? input.cues.map(c => c.text).join('')));
      const bilingual = input.bilingual ? { ...input.bilingual, secondaryStyle: { ...input.bilingual.secondaryStyle } } : undefined;
      if (bilingual) {
        validateCues(input.cues.map(c => ({ ...c, text: c.secondaryText ?? '' })));
        bilingual.secondaryStyle.fontName = resolveBurnFontName(bilingual.secondaryStyle.fontName, /[\u3400-\u9fff]/u.test(input.cues.map(c => c.secondaryText).join('')));
      }
      const report = newFontFallbackReport();
      const resolved = resolveAssFonts(input.ass ?? buildAssDocument(input.cues, style, bilingual, measureStyle, input.frameAspect), style.fontName, [], report);
      // A strict imported ASS may not have its font declarations rewritten.
      // Stop on missing fonts rather than preview one layout and burn another.
      if (input.ass && resolved.content !== input.ass) throw new Error('E_ASS_FONT');
      if (!request.fontsPath) throw new Error('E_FONT_DIR');
      const fonts = await collectFonts(resolved.fontNames, request.fontsPath);
      result = { ass: resolved.content, fonts, style, report: summarizeFontFallbackReport(report) };
    } else throw new Error('E_OPERATION');
    const output = await open(request.outputPath, 'wx');
    try { await output.writeFile(JSON.stringify(result)); await output.sync(); } finally { await output.close(); }
    console.log(JSON.stringify({ protocol: 1, requestId, ok: true }));
  } catch (error) {
    console.log(JSON.stringify({ protocol: 1, requestId, ok: false, error: error instanceof Error ? error.message : String(error) }));
    process.exitCode = 1;
  }
}
