import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import { test } from 'node:test';
import { chromium, firefox } from 'playwright';

// A page whose main landmark holds many controls, each deep in wrappers, the
// way a GitHub README holds anchors and copy buttons. Every visible control is
// certified in each capture, and each one's owner is the main landmark.
const CONTROLS = 40;
const DEPTH = 12;
const url = 'https://capture-cost.invalid/readme';
const control = index => `${'<div class="w">'.repeat(DEPTH)}<button type="button" aria-label="Copy snippet ${index}">Copy</button><span>Snippet ${index} text</span>${'</div>'.repeat(DEPTH)}`;
const body = `<!doctype html><meta charset="utf-8"><style>body{margin:8px;font:12px sans-serif}.w{display:inline}button{padding:1px 4px}</style>
<main><h1>Readme</h1>${Array.from({ length: CONTROLS }, (_, index) => control(index)).join('')}</main>`;

async function fixture(engine, build) {
  const { installPageFeedback } = await import(`../src/${build}/src/agent/page-feedback.js`);
  const browser = await engine.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1200, height: 900 } });
  await context.route('https://capture-cost.invalid/**', route => route.fulfill({ contentType: 'text/html', body }));
  let page;
  class Host {
    static STATE_CHANGE_TOOLS = new Set(['click_ax']);
    isRunning() { return true; }
    _checkAbort() { return false; }
    _throwIfAborted() {}
    _persist() {}
    _effectiveRunMode() { return 'ask'; }
    _appOwnedUserMessage(content, webbrainAppOwnedKind) { return { role: 'user', content, webbrainAppOwnedKind }; }
    _wrapUntrusted(_kind, content) { return content; }
  }
  installPageFeedback(Host);
  const host = new Host();
  const captures = [];
  globalThis.chrome = { runtime: {}, tabs: {
    get: async () => ({ url: page?.url() || url }),
    sendMessage: async (_tab, message) => {
      if (!page) return {};
      const before = await page.evaluate(() => window.qaStyleReads);
      const result = await page.evaluate(message => qaSend(message.action, message.params || {}, message), message);
      if (message.action === 'page_monitor_capture_model') {
        captures.push({ result, styleReads: await page.evaluate(() => window.qaStyleReads) - before });
      }
      return result;
    },
  }, webNavigation: { getAllFrames: async () => [{ frameId: 0, documentId: await page.evaluate(() => String(performance.timeOrigin)), url: page.url() }] } };
  delete globalThis.browser;
  await context.addInitScript(() => {
    // Count style reads: the capture's cost on a deep page is style reads.
    const getComputedStyle = window.getComputedStyle;
    window.qaStyleReads = 0;
    window.getComputedStyle = function countedComputedStyle(...args) {
      window.qaStyleReads += 1;
      return getComputedStyle.apply(this, args);
    };
    const listeners = [];
    const runtime = { getURL: value => value, onMessage: { addListener: fn => listeners.push(fn), removeListener: fn => { const i = listeners.indexOf(fn); if (i >= 0) listeners.splice(i, 1); } },
      async sendMessage(message) {
        if (message.action === 'get_page_monitor_state') return qaState(message.documentToken, String(performance.timeOrigin), location.href);
        if (message.action === 'page_feedback') return qaFeedback(message.feedback, String(performance.timeOrigin));
        return {};
      } };
    window.chrome = { runtime }; window.browser = window.chrome;
    window.qaSend = (action, params = {}, original = {}) => new Promise((resolve, reject) => {
      const timeout = setTimeout(() => reject(new Error(`Missing production content response ${action}`)), 30000);
      let answered = false;
      const respond = result => { answered = true; clearTimeout(timeout); resolve(result); };
      for (const listener of listeners) {
        const async = listener({ ...original, target: 'content', action, params }, {}, respond);
        if (answered || async === true) break;
      }
    });
  });
  for (const file of ['page-monitor-shadow.js', 'rich-text-toolbar-heuristic.js', 'accessibility-tree.js', 'page-monitor.js', 'content.js'])
    await context.addInitScript({ content: await fs.readFile(new URL(`../src/${build}/src/content/${file}`, import.meta.url), 'utf8') });
  page = await context.newPage();
  await page.exposeFunction('qaState', (token, documentId, pageUrl) => host.pageMonitorState({ tab: { id: 1 }, frameId: 0, documentId, url: pageUrl }, token));
  await page.exposeFunction('qaFeedback', (feedback, documentId) => host.observePageFeedback({ tab: { id: 1 }, frameId: 0, documentId }, feedback));
  await page.goto(url);
  await host._beginPageFeedbackRun(1, 'interactive');
  await page.waitForFunction(() => __wbPageMonitor?.active);
  await page.waitForTimeout(200);
  return { browser, page, host, captures };
}

for (const [build, engine] of [['chrome', chromium], ['firefox', firefox]]) {
  test(`${build}: one capture reads each element's style a bounded number of times, however many controls it binds`, { timeout: 60000 }, async () => {
    const f = await fixture(engine, build);
    try {
      const messages = [];
      await f.host._capturePageFeedbackModelState(1, messages);
      const capture = f.captures.find(item => item.result?.ready === true);
      assert.ok(capture, 'capture did not complete');
      assert.equal(capture.result.targetCount, CONTROLS, 'every copy button is bound to the main landmark');
      const elements = await f.page.evaluate(() => document.querySelectorAll('*').length);
      // Once per control the old capture walked every element of main and
      // read the style of each one's ancestors again: about a million reads
      // here. One scan of main per capture keeps it near 75 per element.
      assert.ok(capture.styleReads < 150 * elements, `${capture.styleReads} style reads for ${elements} elements`);
    } finally {
      await f.browser.close();
    }
  });
}
