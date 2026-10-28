import JASSUB from 'jassub';

// Only the mature libass renderer draws subtitles. All user controls remain in
// MyGo, and fonts are supplied by SmartSub through local capability URLs.
// Errors are stable codes; the native side says them in the app's language.
const video = document.getElementById('video');
window.video = video;
window.previewError = '';
window.subtitleReady = false;
let renderer;
let fontKey;
let sequence = Promise.resolve();
video.addEventListener('error', () => { window.previewError = 'unsupported-video'; });
// The native layer owns the player's menu; WebView2's page menu would show
// browser commands in the environment's start-up language.
document.addEventListener('contextmenu', (event) => event.preventDefault());
// JASSUB draws on video frame callbacks. A paused video presents no new frame,
// so the frame already shown when the renderer starts is drawn here.
const drawPaused = () => {
  if (renderer && video.paused && video.readyState >= 2) {
    return renderer.manualRender({ mediaTime: video.currentTime, width: video.videoWidth, height: video.videoHeight }, true);
  }
};
window.setSubtitles = (document) => {
  window.subtitleReady = false;
  sequence = sequence.catch(() => {}).then(async () => {
    window.previewError = '';
    if (renderer && document && fontKey === document.fontKey) {
      await renderer.ready;
      await renderer.renderer.setTrack(document.ass);
      await renderer.resize(true);
      await drawPaused();
      window.subtitleReady = true;
      return;
    }
    if (renderer) { await renderer.destroy(); renderer = undefined; }
    if (!document) return;
    fontKey = document.fontKey;
    renderer = new JASSUB({
      video,
      subContent: document.ass,
      workerUrl: new URL('./worker.js', import.meta.url).href,
      wasmUrl: new URL('./jassub-worker.wasm', import.meta.url).href,
      modernWasmUrl: new URL('./jassub-worker-modern.wasm', import.meta.url).href,
      fonts: document.fonts,
      defaultFont: document.defaultFont,
      queryFonts: false,
      libassMemoryLimit: 64 * 1024 * 1024,
      libassGlyphLimit: 10000,
    });
    await renderer.ready;
    await renderer.resize(true);
    await drawPaused();
    window.subtitleReady = true;
  }).catch(() => { window.previewError = 'subtitle-preview'; });
  return sequence;
};
