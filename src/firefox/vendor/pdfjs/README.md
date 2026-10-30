# pdfjs-dist (vendored)

Mozilla PDF.js, used by `src/agent/pdf-tools.js` to extract text from
PDFs the user is viewing in Firefox. Firefox's built-in PDF viewer is
itself a privileged page that our content scripts can't inject into,
so instead of scraping the viewer's DOM we fetch the PDF binary and
parse it with pdfjs in the background page.

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

`pdf-tools.js` does a lazy dynamic import on the first PDF read:

```js
const pdfjs = await import(browser.runtime.getURL('vendor/pdfjs/index.mjs'));
```

The worker URL is resolved the same way:

```js
pdfjs.GlobalWorkerOptions.workerSrc =
  browser.runtime.getURL('vendor/pdfjs/worker.mjs');
```

The PDF.js entry points and modules are listed in `manifest.json`'s
`web_accessible_resources` so `browser.runtime.getURL` returns a fetchable URL.

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

Keep parity with `src/chrome/vendor/pdfjs/` — both extensions ship the
same pdfjs files. Don't try to bundle pdfjs through any build step;
the project ships extension source as-is.
