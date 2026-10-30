import JASSUB from 'jassub';

// The page only renders media/ASS. Native UI owns controls, menus and layout.
const video = document.getElementById('video');
window.video = video;
let mediaError = '';
let subtitleError = '';
Object.defineProperty(window, 'previewError', { get: () => mediaError || subtitleError });
window.subtitleReady = false;
let renderer;
let fontKey;
let sequence = Promise.resolve();
let revision = 0;
let mediaGeneration = 0;
video.addEventListener('error', () => { mediaError = 'unsupported-video'; });
video.addEventListener('loadeddata', () => { mediaError = ''; });
document.addEventListener('contextmenu', (event) => event.preventDefault());

// A paused first frame needs an explicit subtitle redraw after initialization.
const drawPaused = () => {
  if (renderer && video.paused && video.readyState >= 2) {
    return renderer.manualRender({ mediaTime: video.currentTime, width: video.videoWidth, height: video.videoHeight }, true);
  }
};
window.loadMedia = (source, generation) => {
  if (generation <= mediaGeneration) return;
  mediaGeneration = generation;
  video.pause();
  window.setSubtitles(null);
  mediaError = '';
  if (source) video.src = source;
  else video.removeAttribute('src');
  video.load();
};

// Latest-wins across asynchronous worker startup and first import: obsolete
// subtitle builds never declare readiness/error for another video or style.
// Separate error channels keep a successful ASS update from masking a media
// decode failure, and clear a subtitle error only after a successful retry.
window.setSubtitles = (document) => {
  const current = ++revision;
  window.subtitleReady = false;
  sequence = sequence.catch(() => {}).then(async () => {
    if (current !== revision) return;
    if (renderer && document && fontKey === document.fontKey) {
      await renderer.ready;
      if (current !== revision) return;
      await renderer.renderer.setTrack(document.ass);
    } else {
      if (renderer) { await renderer.destroy(); renderer = undefined; }
      if (current !== revision) return;
      if (!document) { subtitleError = ''; return; }
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
    }
    if (current !== revision) return;
    await renderer.resize(true);
    await drawPaused();
    if (current === revision) { subtitleError = ''; window.subtitleReady = true; }
  }).catch(() => {
    if (current === revision) subtitleError = 'subtitle-preview';
  });
  return sequence;
};
