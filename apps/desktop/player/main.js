import JASSUB from 'jassub';

// Only the mature libass renderer draws subtitles. All user controls remain in
// MyGo, and fonts are supplied by SmartSub through local capability URLs.
const video = document.getElementById('video');
window.video = video;
window.previewError = '';
window.subtitleReady = false;
let renderer;
let fontKey;
let sequence = Promise.resolve();
video.onclick = () => video.paused ? video.play() : video.pause();
video.addEventListener('error', () => { window.previewError = '视频无法直接播放，请转换为 H.264 / AAC MP4 后重试'; });
window.setSubtitles = (document) => {
  window.subtitleReady = false;
  sequence = sequence.catch(() => {}).then(async () => {
    window.previewError = '';
    if (renderer && document && fontKey === document.fontKey) {
      await renderer.ready;
      await renderer.renderer.setTrack(document.ass);
      await renderer.resize(true);
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
    window.subtitleReady = true;
  }).catch(() => { window.previewError = '字幕预览失败，请重试预览'; });
  return sequence;
};
