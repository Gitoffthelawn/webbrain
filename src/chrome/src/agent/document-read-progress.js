import { isReadPageWindowResult } from './read-page-window.js';

// read_page returns a long page a few thousand characters at a time, so a long
// article or README takes several windows, and the Summarize this page action
// asks for all of them. A window that adds text not read before is progress
// through a fixed page, not observation drift, and the delivery checkpoint does
// not count it. The first window of a page still counts, so does a window that
// rereads earlier text, and a run gets a limited number of uncounted windows:
// moving from page to page, or a page too long to finish, still reaches the
// checkpoint.
const MAX_DOCUMENTS = 8;
const MAX_PROGRESS_WINDOWS = 16;
const MIN_NEW_CHARS = 1000;

function documentUrl(value) {
  try {
    const url = new URL(String(value || ''));
    if (!['http:', 'https:', 'file:'].includes(url.protocol)) return '';
    url.hash = '';
    return url.href;
  } catch {
    return '';
  }
}

function deliveredWindow(result) {
  if (!isReadPageWindowResult(result) || result.success === false || result.error
      || result.accessState === 'blocked_by_page_gate') return null;
  const url = documentUrl(result.url);
  const originalLength = Number(result.originalLength);
  const start = Number(result.textOffset);
  const end = start + result.text.length;
  if (!url || !result.text.trim()
      || !Number.isSafeInteger(originalLength) || originalLength <= 0
      || !Number.isSafeInteger(start) || start < 0 || end > originalLength) return null;
  // Live pages change length as timestamps and counters update, so a page is
  // its address and extraction options; positions are compared as they come.
  return { key: `${url}|${result.includeChrome === true}`, start, end, whole: start === 0 && end === originalLength };
}

export function createDocumentReadProgress() {
  return { documents: new Map(), progressWindows: 0 };
}

export function recordDocumentReadProgress(state, result = null) {
  const current = state || createDocumentReadProgress();
  const window = deliveredWindow(result);
  if (!window) return { state: current, madeProgress: false };
  const ranges = current.documents.get(window.key);
  if (!ranges) {
    // A page read in one window leaves nothing further to read.
    if (!window.whole && current.documents.size < MAX_DOCUMENTS) {
      current.documents.set(window.key, [[window.start, window.end]]);
    }
    return { state: current, madeProgress: false };
  }
  let newChars = window.end - window.start;
  for (const [coveredStart, coveredEnd] of ranges) {
    newChars -= Math.max(0, Math.min(window.end, coveredEnd) - Math.max(window.start, coveredStart));
  }
  const merged = [];
  for (const range of [...ranges, [window.start, window.end]].sort((a, b) => a[0] - b[0])) {
    const last = merged.at(-1);
    if (last && range[0] <= last[1]) last[1] = Math.max(last[1], range[1]);
    else merged.push([...range]);
  }
  current.documents.set(window.key, merged);
  // A short last window counts in full; a window that mostly rereads does not.
  const madeProgress = current.progressWindows < MAX_PROGRESS_WINDOWS
    && newChars >= Math.min(MIN_NEW_CHARS, window.end - window.start);
  if (madeProgress) current.progressWindows += 1;
  return { state: current, madeProgress };
}
