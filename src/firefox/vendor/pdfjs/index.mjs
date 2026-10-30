// Page-side entry point for PDF.js. Import this instead of pdf.mjs so the
// polyfills in compat.mjs are installed before PDF.js evaluates.
import './compat.mjs';

export * from './pdf.mjs';
