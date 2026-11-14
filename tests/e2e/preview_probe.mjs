// Test-only observation of the real packaged WebView. The observer delegates
// every update unchanged; readiness and screenshots come from the actual page.
import { writeFile } from 'node:fs/promises';
import assert from 'node:assert/strict';

const [port, mode, output] = process.argv.slice(2);
const pages = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
const page = pages.find(page => page.type === 'page' && page.url.startsWith('http://mygo.localhost/'));
assert(page, 'The isolated media page is missing');
const socket = new WebSocket(page.webSocketDebuggerUrl);
await new Promise((resolve, reject) => {
  socket.addEventListener('open', resolve, { once: true });
  socket.addEventListener('error', reject, { once: true });
});
let serial = 0;
const replies = new Map();
socket.addEventListener('message', event => {
  const message = JSON.parse(event.data);
  const reply = replies.get(message.id);
  if (reply) {
    replies.delete(message.id);
    clearTimeout(reply.timer);
    message.error ? reply.reject(new Error(JSON.stringify(message.error))) : reply.resolve(message.result);
  }
});
const send = (method, params = {}) => new Promise((resolve, reject) => {
  const id = ++serial;
  const timer = setTimeout(() => { replies.delete(id); reject(new Error(`Timed out: ${method}`)); }, 15000);
  replies.set(id, { resolve, reject, timer });
  socket.send(JSON.stringify({ id, method, params }));
});
const evaluate = async expression => {
  const reply = await send('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true });
  assert(!reply.exceptionDetails, JSON.stringify(reply.exceptionDetails));
  return reply.result.value;
};
try {
  if (mode === 'observe') {
    await evaluate(`(() => {
      window.previewTestASS = null;
      const update = window.setSubtitles;
      window.setSubtitles = function(document) {
        window.previewTestASS = document?.ass ?? null;
        return update(document);
      };
    })()`);
  } else {
    const state = await evaluate(`({ready:window.subtitleReady,error:window.previewError,ass:window.previewTestASS,paused:video.paused,time:video.currentTime})`);
    assert(state.ready && !state.error, 'The current subtitle renderer is not ready');
    assert(state.ass?.includes('你好，世界'), 'The preview still contains an older subtitle track');
    assert.equal(state.ass.includes('\u{4e7fd}'), mode === 'missing', 'Preview text does not match the current edit');
    assert(state.paused && state.time > 0.1 && state.time < 1.4, 'The fixture cue is not visible');
    const screenshot = await send('Page.captureScreenshot', { format: 'png' });
    await writeFile(`${output}.png`, Buffer.from(screenshot.data, 'base64'));
    await writeFile(`${output}.json`, JSON.stringify({ ready: state.ready, error: state.error, paused: state.paused, time: state.time, missingCharacter: mode === 'missing' }, null, 2));
  }
  console.log(`PASS: isolated subtitle preview ${mode}`);
} finally {
  socket.close();
}
