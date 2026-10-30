# pdfjs-dist (vendored)

Mozilla PDF.js, used by `src/agent/pdf-tools.js` to extract text from
PDFs the user is viewing in their browser. The Chrome PDF viewer is a
`chrome-extension://` page that our content scripts can't inject into,
so instead of trying to scrape the viewer's DOM we fetch the PDF
binary and parse it with pdfjs in the shared offscreen document.

## Source

- Package: `pdfjs-dist` v5.7.284
- Upstream files: `build/pdf.mjs` + `build/pdf.worker.mjs` (the modern build),
  byte-identical to the npm package
- Origin: <https://github.com/mozilla/pdf.js>
- Upstream license: Apache-2.0 (see `LICENSE` in this folder)

## Files we add

- `compat.mjs`: feature-detected polyfills for the few recent JavaScript
  methods the modern build calls without a fallback (`Map`/`WeakMap`
  `getOrInsertComputed`, `Math.sumPrecise`, `Uint8Array` base64/hex,
  `Response.prototype.bytes`). Each one is installed only when the browser
  lacks it.
- `index.mjs`: page entry point. Loads `compat.mjs`, then re-exports `pdf.mjs`.
- `worker.mjs`: worker entry point. Loads `compat.mjs`, then re-exports
  `pdf.worker.mjs`, so the PDF.js worker gets the same polyfills.

## Why the modern build

The legacy build bundles `core-js`, including old Internet Explorer fallbacks
(an ActiveX/iframe `document.write` path) that Chrome Web Store review rejects
as obfuscated code. The modern build contains no `core-js` at all. With
`compat.mjs`, PDF reading works on Chrome 128+ and Firefox 140+.

## How it's loaded

`src/offscreen/pdf-extraction-host.js` does a lazy dynamic import on the first
PDF read. PDF.js must run in an extension page because the Chrome MV3 service
worker rejects dynamic `import()`:

```js
const pdfjs = await import(chrome.runtime.getURL('vendor/pdfjs/index.mjs'));
```

The worker URL is resolved the same way:

```js
pdfjs.GlobalWorkerOptions.workerSrc =
  chrome.runtime.getURL('vendor/pdfjs/worker.mjs');
```

The Agent's `read_pdf` facade ensures the shared offscreen document, waits for
an explicit ready response, and then sends the PDF URL plus bounded page
options. The host accepts requests only from the extension's background
service worker and only fetches `http:`, `https:`, or `file:` URLs. For Claude
passthrough, it encodes the same fetched bytes before PDF.js transfers the
buffer to its worker, so text extraction and the document attachment cannot
diverge. The PDF.js entry points and modules are listed in `manifest.json`'s
`web_accessible_resources`, so `chrome.runtime.getURL` returns a fetchable URL
without loading the multi-megabyte modules during ordinary service-worker
startup.

## Updating

1. `npm pack pdfjs-dist@latest` (in any scratch directory).
2. `tar xzf pdfjs-dist-*.tgz package/build/pdf.mjs
    package/build/pdf.worker.mjs package/LICENSE`
3. Move them over `pdf.mjs`, `pdf.worker.mjs`, `LICENSE` here.
4. Do not edit the upstream files. If the new version calls another recent
   built-in without a fallback, add a feature-detected polyfill to
   `compat.mjs` instead.
5. Update the version line at the top of this README.
6. Smoke-test by opening a real PDF tab in the loaded extension and
   running a `read_pdf` from the side panel.

Don't try to bundle pdfjs through any build step — the project
ships extension source as-is and pdfjs is already a published
distributable, so re-bundling would duplicate work and risk drift.
