import { expect, test } from 'bun:test';
import { buildAssDocument } from './smartsub/main/helpers/assStyleBuilder';
import { resolveAssFonts } from './smartsub/main/helpers/assFonts';
import { prepareSubtitleFonts } from './smartsub/main/helpers/fontResolver';
import { DEFAULT_STYLE } from './smartsub/renderer/components/subtitleMerge/constants';

// Exercise the complete style-to-font-list path, not only the ASS builder.
// Previously Secondary appeared in ASS but its family never reached libass.
test('both bilingual families and sizes survive named style resets', async () => {
  if (process.platform !== 'win32') return;
  await prepareSubtitleFonts();
  const main = { ...DEFAULT_STYLE, fontName: 'Arial', fontSize: 30 };
  const secondary = { ...DEFAULT_STYLE, fontName: 'Courier New', fontSize: 16, italic: true };
  for (const secondaryFirst of [false, true]) {
    const ass = buildAssDocument([{startMs:0,endMs:2000,text:'Main text',secondaryText:'Secondary text'}],main,{secondaryStyle:secondary,secondaryFirst});
    const resolved = resolveAssFonts(ass,main.fontName);
    expect(resolved.fontNames).toContain('Arial');
    expect(resolved.fontNames).toContain('Courier New');
    expect(resolved.content).toContain('Style: Default,Arial,30,');
    expect(resolved.content).toContain('Style: Secondary,Courier New,16,');
    expect(resolved.content).toContain('{\\rSecondary}Secondary text');
  }
});

test('event style and bare resets select the event font rather than global default', async () => {
  if (process.platform !== 'win32') return;
  await prepareSubtitleFonts();
  const ass = '[V4+ Styles]\nFormat: Name, Fontname\nStyle: Default,Arial\nStyle: Subtitle,Courier New\n[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,0:00:00.00,0:00:02.00,Subtitle,Text{\\fnArial}other{\\r}rest\n';
  const resolved=resolveAssFonts(ass,'Arial');
  expect(resolved.fontNames).toContain('Courier New');
  expect(resolved.content).toContain('{\\r}rest');
});
