// Worker entry point for PDF.js. Use this URL as GlobalWorkerOptions.workerSrc
// so the PDF.js worker gets the same compat.mjs polyfills as the page.
// pdf.worker.mjs starts listening for messages as soon as it evaluates; the
// re-export serves PDF.js's main-thread fallback, which imports workerSrc and
// reads WorkerMessageHandler from it when a real worker cannot start.
import './compat.mjs';

export * from './pdf.worker.mjs';
