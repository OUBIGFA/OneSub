import { createHash } from 'node:crypto';
import { readFile, open, mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { buildAssDocument } from './smartsub/main/helpers/assStyleBuilder';
import { resolveAssFonts } from './smartsub/main/helpers/assFonts';
import { listSubtitleFonts, prepareSubtitleFonts, newFontFallbackReport, summarizeFontFallbackReport, resolveBurnFontName, loadFontData } from './smartsub/main/helpers/fontResolver';
import { DEFAULT_STYLE, STYLE_PRESETS } from './smartsub/renderer/components/subtitleMerge/constants';
import type { SubtitleStyle } from './smartsub/types/subtitleMerge';
import type { SubtitleCue } from './smartsub/main/helpers/subtitleFormats';

// The process adapter owns I/O and validation only. Font selection, presets and
// ASS construction execute SmartSub's modules (font-alias patch is registered).
export function validateCues(cues: SubtitleCue[]) {
  if (!cues.length) throw new Error('没有可合成的字幕');
  for (const cue of cues) {
    if (!cue.text.trim() || !Number.isSafeInteger(cue.startMs) || !Number.isSafeInteger(cue.endMs) || cue.startMs < 0 || cue.endMs <= cue.startMs || Math.round(cue.endMs / 10) <= Math.round(cue.startMs / 10)) {
      throw new Error('字幕文字或时间无效，未删除任何字幕');
    }
  }
}

// Cache only SmartSub's exact selected family/variants, never redistribute a
// system font. Both libass consumers receive these same content-addressed bytes.
export async function collectFonts(names: string[], directory: string) {
  await mkdir(directory, { recursive: true });
  const fonts: { name: string; path: string; sha256: string }[] = [];
  const seen = new Set<string>();
  for (const name of names) {
    const loaded = loadFontData(name);
    if (!loaded) throw new Error(`字体不可用：${name}`);
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
          throw new Error('字体缓存校验失败');
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
      if (Buffer.byteLength(control) > 1024 * 1024) throw new Error('控制消息超过 1 MiB');
    }
    const request = JSON.parse(control);
    requestId = request.requestId;
    if (request.protocol !== 1 || !request.outputPath) throw new Error('样式协议无效');
    await prepareSubtitleFonts();
    let result: unknown;
    if (request.operation === 'catalog') {
      const fontName = process.platform === 'win32' ? 'Microsoft YaHei' : process.platform === 'darwin' ? 'PingFang SC' : 'Noto Sans CJK SC';
      result = { presets: STYLE_PRESETS, defaultStyle: { ...DEFAULT_STYLE, fontName }, fonts: await listSubtitleFonts() };
    } else if (request.operation === 'build' || request.operation === 'resolve') {
      const raw = await readFile(request.inputPath);
      if (raw.byteLength > 64 * 1024 * 1024) throw new Error('样式输入超过 64 MiB');
      const input = JSON.parse(raw.toString('utf8')) as { cues: SubtitleCue[]; ass?: string; style: SubtitleStyle };
      if (request.operation === 'build') validateCues(input.cues);
      else if (!input.ass?.trim()) throw new Error('没有 ASS 文档');
      const style = { ...input.style };
      // Resolve with the exact same installed-font policy as SmartSub; report the
      // resolved name so missing fonts are not silently presented as the requested one.
      style.fontName = resolveBurnFontName(style.fontName, /[\u3400-\u9fff]/u.test(input.ass ?? input.cues.map(c => c.text).join('')));
      const report = newFontFallbackReport();
      const resolved = resolveAssFonts(input.ass ?? buildAssDocument(input.cues, style), style.fontName, [], report);
      // A strict imported ASS may not have its font declarations rewritten.
      // Stop on missing fonts rather than preview one layout and burn another.
      if (input.ass && resolved.content !== input.ass) throw new Error('ASS 字体不可用，请先安装原字体');
      if (!request.fontsPath) throw new Error('缺少字体缓存目录');
      const fonts = await collectFonts(resolved.fontNames, request.fontsPath);
      result = { ass: resolved.content, fonts, style, report: summarizeFontFallbackReport(report) };
    } else throw new Error('未知样式操作');
    const output = await open(request.outputPath, 'wx');
    try { await output.writeFile(JSON.stringify(result)); await output.sync(); } finally { await output.close(); }
    console.log(JSON.stringify({ protocol: 1, requestId, ok: true }));
  } catch (error) {
    console.log(JSON.stringify({ protocol: 1, requestId, ok: false, error: error instanceof Error ? error.message : String(error) }));
    process.exitCode = 1;
  }
}
