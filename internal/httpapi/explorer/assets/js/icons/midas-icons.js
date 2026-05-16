// /explorer/assets/js/icons/midas-icons.js — D33a-spike-2g-impl-2
//
// MIDAS Icon Registry.
//
// Local-only icon registry that exposes the curated 30-icon Lucide
// subset vendored in D33a-spike-2g-impl-1
// (see internal/httpapi/explorer/assets/icons/lucide/). The registry
// is a *foundation* module — no graph or chrome consumer wires it in
// this tranche. The PoC self-authored `_OBJECT_CARD_ICONS` map in
// authority-cytoscape-poc.js is left untouched and remains the
// runtime icon path until D33a-spike-2g-impl-3+.
//
// Public surface (attached to window.MIDASExplorerIcons):
//
//   names                : Array<string>   30 stable MIDAS-facing keys
//   has(name)            : boolean         membership test
//   inlineSvg(name, opts): string          DOM-insertable inline SVG
//   cytoscapeDataURI(n,o): string          data:image/svg+xml;utf8,...
//   _sources             : { name -> meta } provenance + source SVG
//
// Naming uses corrected `posture*` keys (postureDrift / postureDiagnostics).
// The truncated typo previously seen in some draft catalogues is
// corrected here and pinned by the impl-2 tests.
//
// SVG sources are duplicated as string literals below. Provenance
// remains the .svg files under assets/icons/lucide/; the duplication
// is intentional — there is no build pipeline. A test
// (D33aSpike2gImpl2_MidasIconsRegistrySvgMatchesVendoredFiles)
// pins JS↔file equality after whitespace normalization.
//
// No runtime fetches. No external URLs. No dynamic imports.
// The only http(s) URL that appears in this file is the SVG XML
// namespace `http://www.w3.org/2000/svg`, embedded in the verbatim
// SVG source strings.

(function () {
  'use strict';

  // ── Vendored SVG sources ────────────────────────────────────────────
  //
  // Each entry: full SVG string copied verbatim from the corresponding
  // file under assets/icons/lucide/. Trailing file newlines are
  // omitted — the test normalises whitespace.
  //
  // KEEP IN SYNC with the .svg files. The synchronisation pin lives
  // in explorer_d33a_spike2g_impl2_test.go.

  var SVG = {
    // Authority + shared (7)
    'building-2': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 22V4a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v18Z"/><path d="M6 12H4a2 2 0 0 0-2 2v6a2 2 0 0 0 2 2h2"/><path d="M18 9h2a2 2 0 0 1 2 2v9a2 2 0 0 0-2 2h-2"/><path d="M10 6h4"/><path d="M10 10h4"/><path d="M10 14h4"/><path d="M10 18h4"/></svg>',
    'workflow': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="8" height="8" x="3" y="3" rx="2"/><path d="M7 11v4a2 2 0 0 0 2 2h4"/><rect width="8" height="8" x="13" y="13" rx="2"/></svg>',
    'shield-check': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/><path d="m9 12 2 2 4-4"/></svg>',
    'file-check-2': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M4 22h14a2 2 0 0 0 2-2V7l-5-5H6a2 2 0 0 0-2 2v4"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/><path d="m3 15 2 2 4-4"/></svg>',
    'bot': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 8V4H8"/><rect width="16" height="12" x="4" y="8" rx="2"/><path d="M2 14h2"/><path d="M20 14h2"/><path d="M15 13v2"/><path d="M9 13v2"/></svg>',
    'triangle-alert': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3"/><path d="M12 9v4"/><path d="M12 17h.01"/></svg>',
    'arrow-up-from-line': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m18 9-6-6-6 6"/><path d="M12 3v14"/><path d="M5 21h14"/></svg>',

    // Context-only / shared (5)
    'puzzle': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19.439 7.85c-.049.322.059.648.289.878l1.568 1.568c.47.47.706 1.087.706 1.704s-.235 1.233-.706 1.704l-1.611 1.611a.98.98 0 0 1-.837.276c-.47-.07-.802-.48-.879-.95a3 3 0 1 0-3.527 3.527c.48.077.882.41.95.879a.98.98 0 0 1-.276.837l-1.61 1.61a2.404 2.404 0 0 1-1.705.707 2.402 2.402 0 0 1-1.704-.706l-1.568-1.568a1.026 1.026 0 0 0-.877-.29c-.493.074-.84.504-.92.987a3 3 0 1 1-3.236-3.237c.483-.08.913-.427.987-.92a1.026 1.026 0 0 0-.29-.877l-1.568-1.568A2.402 2.402 0 0 1 1.998 12c0-.617.236-1.234.706-1.704L4.23 8.77c.24-.24.581-.353.917-.303.515.077.877.528.954 1.04a3 3 0 1 0 3.434-3.434c-.512-.077-.963-.44-1.04-.954-.05-.336.062-.676.302-.917l1.524-1.523A2.402 2.402 0 0 1 12.022 2c.617 0 1.234.236 1.704.706l1.568 1.568c.23.23.556.338.877.29.493-.074.84-.504.92-.987a3 3 0 1 1 3.236 3.237c-.483.08-.913.427-.987.92Z"/></svg>',
    'route': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="6" cy="19" r="3"/><path d="M9 19h8.5a3.5 3.5 0 0 0 0-7h-11a3.5 3.5 0 0 1 0-7H15"/><circle cx="18" cy="5" r="3"/></svg>',
    'cpu': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="16" height="16" x="4" y="4" rx="2"/><rect width="6" height="6" x="9" y="9"/><path d="M15 2v2"/><path d="M15 20v2"/><path d="M2 15h2"/><path d="M2 9h2"/><path d="M20 15h2"/><path d="M20 9h2"/><path d="M9 2v2"/><path d="M9 20v2"/></svg>',
    'target': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><circle cx="12" cy="12" r="6"/><circle cx="12" cy="12" r="2"/></svg>',
    'link-2': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 17H7A5 5 0 0 1 7 7h2"/><path d="M15 7h2a5 5 0 1 1 0 10h-2"/><line x1="8" x2="16" y1="12" y2="12"/></svg>',

    // Workbench chrome (6)
    'refresh-cw': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M3 21v-5h5"/></svg>',
    'settings': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/></svg>',
    'info': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M12 16v-4"/><path d="M12 8h.01"/></svg>',
    'circle-help': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/><path d="M12 17h.01"/></svg>',
    'external-link': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 3h6v6"/><path d="M10 14 21 3"/><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/></svg>',
    'download': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" x2="12" y1="15" y2="3"/></svg>',

    // Lifecycle (5)
    'circle-dashed': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.1 2.18a9.93 9.93 0 0 1 3.8 0"/><path d="M17.6 3.71a9.95 9.95 0 0 1 2.69 2.7"/><path d="M21.82 10.1a9.93 9.93 0 0 1 0 3.8"/><path d="M20.29 17.6a9.95 9.95 0 0 1-2.7 2.69"/><path d="M13.9 21.82a9.94 9.94 0 0 1-3.8 0"/><path d="M6.4 20.29a9.95 9.95 0 0 1-2.69-2.7"/><path d="M2.18 13.9a9.93 9.93 0 0 1 0-3.8"/><path d="M3.71 6.4a9.95 9.95 0 0 1 2.7-2.69"/></svg>',
    'eye': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2.062 12.348a1 1 0 0 1 0-.696 10.75 10.75 0 0 1 19.876 0 1 1 0 0 1 0 .696 10.75 10.75 0 0 1-19.876 0"/><circle cx="12" cy="12" r="3"/></svg>',
    'circle-check': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="m9 12 2 2 4-4"/></svg>',
    'archive': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="20" height="5" x="2" y="3" rx="1"/><path d="M4 8v11a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8"/><path d="M10 12h4"/></svg>',
    'archive-x': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="20" height="5" x="2" y="3" rx="1"/><path d="M4 8v11a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8"/><path d="m9.5 17 5-5"/><path d="m9.5 12 5 5"/></svg>',

    // Severity / state (3)
    'octagon-alert': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2.586 16.726A2 2 0 0 1 2 15.312V8.688a2 2 0 0 1 .586-1.414l4.688-4.688A2 2 0 0 1 8.688 2h6.624a2 2 0 0 1 1.414.586l4.688 4.688A2 2 0 0 1 22 8.688v6.624a2 2 0 0 1-.586 1.414l-4.688 4.688a2 2 0 0 1-1.414.586H8.688a2 2 0 0 1-1.414-.586z"/><path d="M12 8v4"/><path d="M12 16h.01"/></svg>',
    'circle-x': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="m15 9-6 6"/><path d="m9 9 6 6"/></svg>',
    'circle-pause': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="10" x2="10" y1="15" y2="9"/><line x1="14" x2="14" y1="15" y2="9"/></svg>',

    // Audit / integrity (2)
    'lock': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="18" height="11" x="3" y="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>',
    'lock-open': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="18" height="11" x="3" y="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 9.9-1"/></svg>',

    // Posture (2) — corrected `posture*` naming.
    'trending-down': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="22 17 13.5 8.5 8.5 13.5 2 7"/><polyline points="16 17 22 17 22 11"/></svg>',
    'stethoscope': '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M11 2v2"/><path d="M5 2v2"/><path d="M5 3H4a2 2 0 0 0-2 2v4a6 6 0 0 0 12 0V5a2 2 0 0 0-2-2h-1"/><path d="M8 15a6 6 0 0 0 12 0v-3"/><circle cx="20" cy="10" r="2"/></svg>'
  };

  // ── MIDAS-facing name → Lucide filename ─────────────────────────────
  //
  // Catalogue order: authority, context, chrome, lifecycle, severity,
  // audit, posture. Stable for tests and for downstream consumers.

  var CATALOGUE = [
    // Authority Graph + shared
    { name: 'authorityBusinessService',  lucide: 'building-2',          category: 'authority' },
    { name: 'authorityDecisionSurface',  lucide: 'workflow',            category: 'authority' },
    { name: 'authorityProfile',          lucide: 'shield-check',        category: 'authority' },
    { name: 'authorityGrant',            lucide: 'file-check-2',        category: 'authority' },
    { name: 'authorityAgent',            lucide: 'bot',                 category: 'authority' },
    { name: 'authorityFailModePolicy',   lucide: 'triangle-alert',      category: 'authority' },
    { name: 'authorityEscalationTarget', lucide: 'arrow-up-from-line',  category: 'authority' },
    // Context-only
    { name: 'contextCapability',         lucide: 'puzzle',              category: 'context' },
    { name: 'contextProcess',            lucide: 'route',               category: 'context' },
    { name: 'contextAiSystem',           lucide: 'cpu',                 category: 'context' },
    { name: 'contextCoverage',           lucide: 'target',              category: 'context' },
    { name: 'contextAiSystemBinding',    lucide: 'link-2',              category: 'context' },
    // Workbench chrome
    { name: 'graphRefresh',              lucide: 'refresh-cw',          category: 'chrome' },
    { name: 'chromeSettings',            lucide: 'settings',            category: 'chrome' },
    { name: 'chromeInfo',                lucide: 'info',                category: 'chrome' },
    { name: 'chromeHelp',                lucide: 'circle-help',         category: 'chrome' },
    { name: 'chromeExternal',            lucide: 'external-link',       category: 'chrome' },
    { name: 'chromeDownload',            lucide: 'download',            category: 'chrome' },
    // Lifecycle
    { name: 'lifecycleDraft',            lucide: 'circle-dashed',       category: 'lifecycle' },
    { name: 'lifecycleReview',           lucide: 'eye',                 category: 'lifecycle' },
    { name: 'lifecycleActive',           lucide: 'circle-check',        category: 'lifecycle' },
    { name: 'lifecycleDeprecated',       lucide: 'archive',             category: 'lifecycle' },
    { name: 'lifecycleRetired',          lucide: 'archive-x',           category: 'lifecycle' },
    // Severity / state
    { name: 'severityCritical',          lucide: 'octagon-alert',       category: 'severity' },
    { name: 'stateBlocked',              lucide: 'circle-x',            category: 'severity' },
    { name: 'stateSuspended',            lucide: 'circle-pause',        category: 'severity' },
    // Audit / integrity
    { name: 'auditIntegrityVerified',    lucide: 'lock',                category: 'audit' },
    { name: 'auditIntegrityBroken',      lucide: 'lock-open',           category: 'audit' },
    // Posture (corrected `posture*` naming — see top-of-file comment)
    { name: 'postureDrift',              lucide: 'trending-down',       category: 'posture' },
    { name: 'postureDiagnostics',        lucide: 'stethoscope',         category: 'posture' }
  ];

  // ── Build _sources index ────────────────────────────────────────────

  function extractInner(svg) {
    // Strip the opening <svg ...> tag and the closing </svg>.
    var openEnd = svg.indexOf('>');
    var closeStart = svg.lastIndexOf('</svg>');
    if (openEnd < 0 || closeStart < 0 || closeStart < openEnd) return '';
    return svg.slice(openEnd + 1, closeStart);
  }

  var SOURCES = {};
  var NAMES = [];
  for (var i = 0; i < CATALOGUE.length; i++) {
    var entry = CATALOGUE[i];
    var raw = SVG[entry.lucide];
    if (!raw) continue;
    SOURCES[entry.name] = {
      lucide: entry.lucide,
      file: entry.lucide + '.svg',
      category: entry.category,
      svg: raw,
      inner: extractInner(raw)
    };
    NAMES.push(entry.name);
  }

  // ── Sanitisation helpers ────────────────────────────────────────────
  //
  // The inlineSvg + cytoscapeDataURI helpers accept caller-supplied
  // attribute values. We must defensively reject content that could
  // break out of an attribute or inject script. Guards:
  //
  //   - escapeText: HTML-encode <, >, &, ", ' for use inside <title>.
  //   - sanitiseClassName: limit to a conservative character set.
  //   - sanitiseStroke: accept currentColor / var(--...) / #hex /
  //                     simple named colours; reject anything with
  //                     <, >, ", ', url(, javascript:, or
  //                     event-handler tokens (onload, onclick, …).
  //   - sanitiseLength: accept positive numbers or simple CSS lengths.
  //
  // If an option is invalid, fall back to the default rather than
  // throwing.

  function escapeText(s) {
    return String(s).replace(/[&<>"']/g, function (c) {
      return ({
        '&': '&amp;',
        '<': '&lt;',
        '>': '&gt;',
        '"': '&quot;',
        "'": '&#39;'
      })[c];
    });
  }

  // Patterns rejected anywhere inside a caller-supplied attribute:
  // any of <, >, ", ', the substrings javascript:, url(, onload, onclick,
  // onerror, onfocus, onmouseover. These pin the safety guards.
  var UNSAFE_ATTR_PATTERN = /[<>"']|javascript:|url\(|onload|onclick|onerror|onfocus|onmouseover/i;

  function sanitiseAttribute(value) {
    var str = String(value == null ? '' : value);
    if (UNSAFE_ATTR_PATTERN.test(str)) return null;
    return str;
  }

  function sanitiseClassName(value) {
    if (value == null) return '';
    var str = String(value);
    if (UNSAFE_ATTR_PATTERN.test(str)) return '';
    // Conservative character set: ASCII letters, digits, dash,
    // underscore, space. Anything else → reject.
    if (!/^[\w\- ]*$/.test(str)) return '';
    return str;
  }

  function sanitiseStroke(value, fallback) {
    if (value == null) return fallback;
    var str = String(value);
    if (UNSAFE_ATTR_PATTERN.test(str)) return fallback;
    if (str === 'currentColor') return str;
    // CSS custom property: var(--token) — but we already rejected url(.
    // Allow var(--name) explicitly.
    if (/^var\(--[A-Za-z0-9_-]+\)$/.test(str)) return str;
    // Hex colour: #abc / #aabbcc / #aabbccdd.
    if (/^#[0-9a-fA-F]{3,8}$/.test(str)) return str;
    // Simple named colour, e.g. `red`, `slategray`.
    if (/^[a-zA-Z]{3,32}$/.test(str)) return str;
    return fallback;
  }

  function sanitiseLength(value, fallback) {
    if (value == null) return fallback;
    if (typeof value === 'number' && isFinite(value) && value >= 0) {
      return value;
    }
    var str = String(value);
    if (UNSAFE_ATTR_PATTERN.test(str)) return fallback;
    // Plain number (e.g. "24"), or number+unit (e.g. "1.5em", "32px",
    // "100%"). No whitespace, no calc(), no var().
    if (/^[0-9]+(\.[0-9]+)?(px|em|rem|%|pt|vw|vh)?$/.test(str)) {
      return str;
    }
    return fallback;
  }

  // ── Public API ──────────────────────────────────────────────────────

  function has(name) {
    if (typeof name !== 'string') return false;
    return Object.prototype.hasOwnProperty.call(SOURCES, name);
  }

  // inlineSvg(name, opts)
  //
  // Returns an inline SVG string suitable for `el.innerHTML = ...`.
  // Unknown names return '' (the empty string), never throw. Callers
  // can use `has(name)` to disambiguate from a deliberately empty
  // value.
  function inlineSvg(name, opts) {
    if (!has(name)) return '';
    opts = opts || {};
    var src = SOURCES[name];

    var size = sanitiseLength(opts.size, 24);
    var stroke = sanitiseStroke(opts.stroke, 'currentColor');
    var strokeWidth = sanitiseLength(opts.strokeWidth, 2);
    var className = sanitiseClassName(opts.className);

    var hasTitle = opts.title != null && String(opts.title).length > 0;
    var titleText = hasTitle ? escapeText(opts.title) : '';
    var ariaHiddenFlag = hasTitle ? false : (opts.ariaHidden !== false);

    var attrs =
      'xmlns="http://www.w3.org/2000/svg"' +
      ' width="' + size + '"' +
      ' height="' + size + '"' +
      ' viewBox="0 0 24 24"' +
      ' fill="none"' +
      ' stroke="' + stroke + '"' +
      ' stroke-width="' + strokeWidth + '"' +
      ' stroke-linecap="round"' +
      ' stroke-linejoin="round"';
    if (className) {
      attrs += ' class="' + className + '"';
    }
    if (hasTitle) {
      attrs += ' role="img" aria-hidden="false"';
      return '<svg ' + attrs + '><title>' + titleText + '</title>' + src.inner + '</svg>';
    }
    attrs += ' aria-hidden="' + (ariaHiddenFlag ? 'true' : 'false') + '"';
    return '<svg ' + attrs + '>' + src.inner + '</svg>';
  }

  // cytoscapeDataURI(name, opts)
  //
  // Returns a data URI suitable for Cytoscape `background-image`.
  // Cytoscape does NOT propagate `currentColor`, so the default stroke
  // here is a concrete neutral colour (#e2e8f0) rather than
  // `currentColor`. Callers should pass a per-node stroke colour for
  // theme-aware rendering.
  //
  // Unknown names return '' (matches inlineSvg).
  function cytoscapeDataURI(name, opts) {
    if (!has(name)) return '';
    opts = opts || {};
    var src = SOURCES[name];

    var size = sanitiseLength(opts.size, 24);
    var stroke = sanitiseStroke(opts.stroke, '#e2e8f0');
    var strokeWidth = sanitiseLength(opts.strokeWidth, 2);

    var svg =
      '<svg xmlns="http://www.w3.org/2000/svg"' +
      ' width="' + size + '"' +
      ' height="' + size + '"' +
      ' viewBox="0 0 24 24"' +
      ' fill="none"' +
      ' stroke="' + stroke + '"' +
      ' stroke-width="' + strokeWidth + '"' +
      ' stroke-linecap="round"' +
      ' stroke-linejoin="round">' +
      src.inner +
      '</svg>';

    // encodeURIComponent encodes `<`, `>`, `"`, `'`, whitespace, and the
    // xmlns URL (so the data URI carries no raw `<svg` or unencoded URL).
    return 'data:image/svg+xml;utf8,' + encodeURIComponent(svg);
  }

  // ── Attach to window ───────────────────────────────────────────────

  window.MIDASExplorerIcons = {
    names: NAMES.slice(),
    has: has,
    inlineSvg: inlineSvg,
    cytoscapeDataURI: cytoscapeDataURI,
    _sources: SOURCES,
    // Sanitisation helpers exposed for unit-testability and to make
    // the safety surface inspectable from the console. Internal —
    // consumers should not rely on the shape of these helpers.
    _escapeText: escapeText,
    _sanitiseAttribute: sanitiseAttribute,
    _sanitiseClassName: sanitiseClassName,
    _sanitiseStroke: sanitiseStroke,
    _sanitiseLength: sanitiseLength
  };
})();
