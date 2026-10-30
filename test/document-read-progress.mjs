import assert from 'node:assert/strict';
import { test } from 'node:test';
import { withFixedChoiceAnswers } from './semantic-decision-stub.mjs';

const area = { get: async () => ({}), set: async () => {}, remove: async () => {} };
const api = {
  storage: { local: area, session: area },
  runtime: { getURL: value => `extension://test/${value}`, sendMessage: async () => ({}) },
  tabs: { get: async id => ({ id, url: 'https://example.com/', title: 'Example' }), sendMessage: async () => ({}) },
};
globalThis.chrome = api;
globalThis.browser = api;

const PAGE_URL = 'https://github.com/esokullu/openbonsai';
const SUMMARIZE = 'Use read_page first for the current long-form page. While read_page returns hasMore:true, call it again with its exact continuationArgs until hasMore is false. Then summarize the page in 5-8 concise bullet points and include any action items. If you stop reading early, say which part of the page the summary covers.';
const call = (id, name, args) => ({ toolCalls: [{ id, function: { name, arguments: JSON.stringify(args) } }] });

function makeAskAgent(Agent, next) {
  const answered = withFixedChoiceAnswers(next, {});
  const provider = {
    name: 'summarize regression', model: 'test', promptTier: 'mid', contextWindow: 128000,
    supportsTools: true, supportsVision: false,
    chat: (messages, options) => answered(messages, options),
    async *chatStream(messages) {
      const response = await next(messages);
      if (response.content) yield { type: 'text', content: response.content };
      if (response.toolCalls) yield { type: 'tool_call', content: response.toolCalls.map((item, index) => ({ ...item, index })) };
      yield { type: 'done' };
    },
  };
  const agent = new Agent({ getActive: () => provider, getProvider: () => provider, getVisionProvider: async () => null });
  for (const name of ['_hydrate', '_persistSubmittedTurn', '_endTraceRun', '_manageContext', '_maybeEmitAskModeHandoff',
    '_maybeReinjectAdapter']) agent[name] = async () => {};
  agent._persist = () => {};
  agent._persistNow = async () => ({ ok: true });
  agent._startTraceRun = async () => null;
  agent._beginReadCompleteness = async () => null;
  agent._checkCostAllowance = async () => null;
  agent._recordCostUsage = async () => null;
  agent._currentUrl = async () => PAGE_URL;
  agent._getTabUrlTitle = async () => ({ tabUrl: PAGE_URL, tabTitle: 'esokullu/openbonsai' });
  agent._enrichUserMessageWithCurrentPage = async (_tab, _messages, content) => ({ role: 'user', content });
  agent._plannerMode = () => 'off';
  agent._shouldAutoScreenshot = () => false;
  agent._pageFeedbackIdleMs = 0;
  agent._completionDoneBlock = () => null;
  agent.maxSteps = Infinity;
  return agent;
}

// The README from the trace that prompted this: 42,833 characters, read in
// 6,000-character windows by the Summarize this page action.
const README_LENGTH = 42833;

for (const build of ['chrome', 'firefox']) {
  const { Agent } = await import(`../src/${build}/src/agent/agent.js`);
  const { recordDocumentReadProgress } = await import(`../src/${build}/src/agent/document-read-progress.js`);
  const { applyReadPageWindow } = await import(`../src/${build}/src/agent/read-page-window.js`);

  const page = (length = README_LENGTH, { url = PAGE_URL, includeChrome = false } = {}) => ({
    url, title: 'esokullu/openbonsai', text: 'w'.repeat(length), includeChrome,
    links: [{ text: 'Code', href: url }], forms: [], shadowDOM: [],
  });
  const read = (offset, { limit = 6000, length, url, includeChrome } = {}) =>
    applyReadPageWindow(page(length, { url, includeChrome }), { offset, limit, includeChrome });

  test(`${build}: each new read_page window of one page is progress, the first and rereads are not`, () => {
    let state;
    const record = (offset, options) => {
      const recorded = recordDocumentReadProgress(state, read(offset, options));
      state = recorded.state;
      return recorded.madeProgress;
    };
    assert.equal(record(0), false, 'the first window is an ordinary observation');
    for (let offset = 6000; offset < 42000; offset += 6000) {
      assert.equal(record(offset), true, `window at ${offset} adds new text`);
    }
    assert.equal(record(42000), true, 'the short last window adds new text');
    assert.equal(record(42000), false, 'rereading the last window is not progress');
    assert.equal(record(0), false, 'starting over is not progress');
    assert.equal(record(3000), false, 'a window inside text already read is not progress');
  });

  test(`${build}: pages are told apart by address and extraction options`, () => {
    let recorded = recordDocumentReadProgress(undefined, read(0));
    recorded = recordDocumentReadProgress(recorded.state, read(6000, { includeChrome: true }));
    assert.equal(recorded.madeProgress, false, 'a read with page chrome is another document');
    recorded = recordDocumentReadProgress(recorded.state, read(6000, { url: 'https://github.com/esokullu/other' }));
    assert.equal(recorded.madeProgress, false, 'another address is another document');
    recorded = recordDocumentReadProgress(recorded.state, read(6000, { url: `${PAGE_URL}#readme` }));
    assert.equal(recorded.madeProgress, true, 'a fragment is the same page');
  });

  test(`${build}: a page read in one window, a blocked page or a failed read is never progress`, () => {
    let recorded = recordDocumentReadProgress(undefined, read(0, { length: 3000 }));
    recorded = recordDocumentReadProgress(recorded.state, read(0, { length: 3000 }));
    assert.equal(recorded.madeProgress, false, 'a whole page has nothing further to read');
    recorded = recordDocumentReadProgress(undefined, read(0));
    const blocked = { ...read(6000), accessState: 'blocked_by_page_gate' };
    assert.equal(recordDocumentReadProgress(recorded.state, blocked).madeProgress, false);
    assert.equal(recordDocumentReadProgress(recorded.state, { success: false, error: 'No tab' }).madeProgress, false);
    assert.equal(recordDocumentReadProgress(recorded.state, { success: true }).madeProgress, false);
  });

  test(`${build}: a run gets sixteen uncounted windows and eight pages`, () => {
    let state;
    const total = 600000;
    let progress = 0;
    for (let offset = 0; offset < 30 * 6000; offset += 6000) {
      const recorded = recordDocumentReadProgress(state, read(offset, { length: total }));
      state = recorded.state;
      if (recorded.madeProgress) progress += 1;
    }
    assert.equal(progress, 16);

    state = undefined;
    for (let index = 0; index < 9; index++) {
      const url = `https://example.com/article-${index}`;
      state = recordDocumentReadProgress(state, read(0, { url })).state;
      const next = recordDocumentReadProgress(state, read(6000, { url }));
      assert.equal(next.madeProgress, index < 8, `page ${index + 1}`);
      state = next.state;
    }
  });

  test(`${build}: Summarize this page reads a long README to the end without forced delivery`, () => {
    const agent = new Agent({});
    const tab = `${build}-summarize`;
    const enforced = { enforceTerminal: true };
    const checks = [];
    for (let offset = 0; offset < README_LENGTH; offset += 6000) {
      const args = offset ? { continuationArgs: { includeChrome: false, offset, limit: 6000 } } : { offset: 0, limit: 6000, includeChrome: false };
      checks.push(agent._checkDeliveryObservationStreak(tab, 'read_page', args, read(offset), enforced));
    }
    assert.equal(checks.length, 8);
    assert.deepEqual(checks.map(check => check.kind), Array(8).fill('none'));
    assert.equal(agent.deliveryObservationStreaks.get(tab), 1, 'only the first window counts');
    agent._clearRunLoopState(tab);
    assert.equal(agent.deliveryDocumentReads.has(tab), false, 'a new run starts its own page coverage');
  });

  test(`${build}: rereading, other observations and a page past the window budget still reach the checkpoint`, () => {
    const agent = new Agent({});
    const enforced = { enforceTerminal: true };
    const rereadTab = `${build}-reread`;
    let check;
    for (let index = 0; index < 8; index++) check = agent._checkDeliveryObservationStreak(rereadTab, 'read_page', {}, read(0), enforced);
    assert.equal(check.kind, 'deliver', 'the same window read eight times');

    const mixedTab = `${build}-mixed`;
    for (let index = 0; index < 3; index++) agent._checkDeliveryObservationStreak(mixedTab, 'scroll', {}, { success: true }, enforced);
    agent._checkDeliveryObservationStreak(mixedTab, 'read_page', {}, read(0), enforced);
    assert.equal(agent.deliveryObservationStreaks.get(mixedTab), 4);
    assert.equal(agent._checkDeliveryObservationStreak(mixedTab, 'read_page', {}, read(6000), enforced).kind, 'none');
    assert.equal(agent.deliveryObservationStreaks.get(mixedTab), 4, 'a new window neither counts nor clears earlier observations');

    const longTab = `${build}-long`;
    const kinds = [];
    for (let offset = 0; kinds.at(-1) !== 'deliver' && offset < 100 * 6000; offset += 6000) {
      kinds.push(agent._checkDeliveryObservationStreak(longTab, 'read_page', {}, read(offset, { length: 600000 }), enforced).kind);
    }
    assert.equal(kinds.length, 24, 'one counted window, sixteen uncounted, then seven counted');
    assert.equal(kinds.filter(kind => kind === 'nudge').length, 1);
  });

  // The traced run: eight windows, the last one ending the page, then done.
  // Before, the eighth window forced a done-only turn and a partial outcome.
  for (const streaming of [false, true]) test(`${build}: ${streaming ? 'stream' : 'chat'} Ask run summarizes the whole README as done, not partial`, async () => {
    const tab = `${build}-ask-summary-${streaming}`, updates = [], offsets = [];
    const summary = '- OpenBonsai compresses dense models to ternary weights.';
    let requests = 0;
    const next = async () => {
      requests += 1;
      if (requests === 1) return call('read-1', 'read_page', { offset: 0, limit: 6000, includeChrome: false });
      if (requests <= 8) {
        return call(`read-${requests}`, 'read_page', { continuationArgs: { includeChrome: false, offset: (requests - 1) * 6000, limit: 6000 } });
      }
      return call('finished', 'done', { summary });
    };
    const agent = makeAskAgent(Agent, next);
    agent.executeTool = async (_tab, name, args) => {
      if (name === 'done') return { done: true, success: true, summary: args.summary };
      assert.equal(name, 'read_page');
      const offset = Number(args.offset ?? args.continuationArgs?.offset ?? 0);
      offsets.push(offset);
      return read(offset);
    };
    const update = (type, data) => updates.push({ type, data });
    const options = { detachedRequestId: `summary-${build}-${streaming}`, askStreamingEnabled: streaming };
    const result = streaming
      ? await agent.processMessageStream(tab, SUMMARIZE, update, 'ask', options)
      : await agent.processMessage(tab, SUMMARIZE, update, 'ask', [], options);
    const warnings = updates.filter(entry => entry.type === 'warning').map(entry => entry.data?.message);
    assert.deepEqual(offsets, [0, 6000, 12000, 18000, 24000, 30000, 36000, 42000]);
    assert.equal(requests, 9, JSON.stringify(warnings));
    assert.equal(String(result).trim(), summary);
    assert.equal(warnings.some(message => /observation tools are now stopping|DELIVERY/.test(String(message))), false, JSON.stringify(warnings));
  });
}
