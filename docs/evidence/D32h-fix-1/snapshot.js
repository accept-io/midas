// /docs/evidence/D32h-fix-1/snapshot.js
//
// Authority Graph runtime DOM snapshot for the D32h-fix-1 browser-
// verification loop. Paste the IIFE body into Chrome DevTools (any
// Chromium-based browser) while viewing the Authority Graph for the
// service under test. The snippet:
//
//   • collects every rendered .gmap-node (id, kind, projection-kind,
//     selected flag, posture data-*, bounding box, computed colours);
//   • collects every #gmap-svg path.gmap-connector (src / dst / kind /
//     d attribute);
//   • collects canvas / scene / svg geometry and the coordinate
//     contract (dataset.baseWidth + viewBox);
//   • collects the scroll container's bounds and scroll offsets;
//   • locates the bottom rail (Drift Analytics) and flags nodes whose
//     bounding box intersects it;
//   • collects selected-node sub-element computed styles so we can
//     diagnose the greyed / corrupted selected-card symptom;
//   • collects authority layer-chip toggle state.
//
// The result is JSON-stringified, written to console, and also written
// to the clipboard if the page is focused so the operator can paste
// straight into the deliverable / chat.
//
// No mutations. No network. Safe to run on any page; non-graph pages
// will produce a sparse snapshot.
//
// Usage:
//   1. Load http://localhost:8080/explorer in your browser, log in.
//   2. Pick the service under test (Authority Graph Showcase first;
//      then Retail Banking; then one of Consumer Lending / Merchant
//      Services; finally a Context Graph render of any service for
//      regression).
//   3. Wait for the canvas to paint (scheduleFitToView completes).
//   4. Open DevTools → Console → paste this snippet → enter.
//   5. The snapshot is copied to clipboard. Paste it back to me as a
//      single JSON blob.

(() => {
  const out = {
    meta: {},
    canvas: {},
    scene: {},
    svg: {},
    viewport: {},
    nodes: [],
    connectors: [],
    selected: null,
    bottomRail: null,
    bodyRails: [],
    layers: {},
    spec: {},
    notes: [],
  };
  const $ = (s, ctx = document) => ctx.querySelector(s);
  const $$ = (s, ctx = document) => Array.from(ctx.querySelectorAll(s));
  const rectOf = (el) => {
    const r = el.getBoundingClientRect();
    return {
      top:    Math.round(r.top),
      left:   Math.round(r.left),
      bottom: Math.round(r.bottom),
      right:  Math.round(r.right),
      width:  Math.round(r.width),
      height: Math.round(r.height),
    };
  };

  // ── Meta ──────────────────────────────────────────────────────────
  out.meta.url = location.href;
  out.meta.hash = location.hash;
  out.meta.now = new Date().toISOString();
  out.meta.devicePixelRatio = window.devicePixelRatio;
  out.meta.innerWidth = window.innerWidth;
  out.meta.innerHeight = window.innerHeight;
  out.meta.activeLens = (() => {
    try { return window.MIDASExplorerStore.getState().selectedGraphLens; }
    catch (_) { return null; }
  })();
  out.meta.userAgent = navigator.userAgent;

  // ── Canvas / scene / svg ──────────────────────────────────────────
  const canvas = document.getElementById('gmap-canvas');
  const scene  = document.getElementById('gmap-scene');
  const svg    = document.getElementById('gmap-svg');
  if (canvas) {
    const cs = getComputedStyle(canvas);
    out.canvas = {
      datasetBaseWidth: canvas.dataset.baseWidth || null,
      styleWidth:       canvas.style.width || '',
      styleMinWidth:    canvas.style.minWidth || '',
      styleHeight:      canvas.style.height || '',
      computedWidth:    cs.width,
      computedHeight:   cs.height,
      scrollWidth:      canvas.scrollWidth,
      scrollHeight:     canvas.scrollHeight,
      clientWidth:      canvas.clientWidth,
      clientHeight:     canvas.clientHeight,
      rect:             rectOf(canvas),
    };
  } else {
    out.notes.push('NO #gmap-canvas — wrong page or graph not rendered?');
  }
  if (scene) {
    out.scene = {
      transform:        scene.style.transform || '',
      styleWidth:       scene.style.width || '',
      styleHeight:      scene.style.height || '',
      computedWidth:    getComputedStyle(scene).width,
      computedHeight:   getComputedStyle(scene).height,
      rect:             rectOf(scene),
    };
  }
  if (svg) {
    out.svg = {
      viewBox:          svg.getAttribute('viewBox') || '',
      width:            svg.getAttribute('width') || '',
      height:           svg.getAttribute('height') || '',
      computedWidth:    getComputedStyle(svg).width,
      computedHeight:   getComputedStyle(svg).height,
      rect:             rectOf(svg),
    };
  }

  // ── Scroll container ──────────────────────────────────────────────
  const scrollEl = document.getElementsByClassName('governance-map-canvas-scroll')[0];
  if (scrollEl) {
    out.viewport = {
      scrollLeft:   scrollEl.scrollLeft,
      scrollTop:    scrollEl.scrollTop,
      scrollWidth:  scrollEl.scrollWidth,
      scrollHeight: scrollEl.scrollHeight,
      clientWidth:  scrollEl.clientWidth,
      clientHeight: scrollEl.clientHeight,
      rect:         rectOf(scrollEl),
    };
  }

  // ── Nodes ─────────────────────────────────────────────────────────
  const nodes = $$('.gmap-node');
  out.meta.nodeCount = nodes.length;
  nodes.forEach((n) => {
    const cs = getComputedStyle(n);
    const labelEl  = n.querySelector('.gmap-node-label, .gmap-node-kind');
    const nameEl   = n.querySelector('.gmap-node-name, .gmap-node-title');
    const metaEl   = n.querySelector('.gmap-node-meta');
    const badgesEl = n.querySelectorAll('.gmap-node-badge, .authority-badge, [class*="badge"]');
    const labelCs  = labelEl ? getComputedStyle(labelEl) : null;
    const nameCs   = nameEl  ? getComputedStyle(nameEl)  : null;
    out.nodes.push({
      id:            n.dataset.nodeId || '',
      kind:          n.dataset.nodeKind || '',
      projectionKind: n.dataset.projectionKind || '',
      cls:           n.className || '',
      selected:      n.classList.contains('selected'),
      hidden:        cs.display === 'none' || cs.visibility === 'hidden',
      // Posture data-* from D32f-impl-1.
      diagnosticSeverity: n.dataset.diagnosticSeverity || '',
      fmpStatus:          n.dataset.fmpStatus || '',
      agentStatus:        n.dataset.agentStatus || '',
      profileStatus:      n.dataset.profileStatus || '',
      grantStatus:        n.dataset.grantStatus || '',
      // Inline-style coordinates from the layout planner.
      styleLeft:     n.style.left,
      styleTop:      n.style.top,
      // Computed coords (post any transform).
      computedLeft:  cs.left,
      computedTop:   cs.top,
      // Bounding rect in viewport coords.
      rect:          rectOf(n),
      // Visible text for human readability.
      labelText:     labelEl ? labelEl.textContent.trim().slice(0, 60) : '',
      nameText:      nameEl  ? nameEl.textContent.trim().slice(0, 80)  : '',
      metaText:      metaEl  ? metaEl.textContent.trim().slice(0, 80)  : '',
      badgeCount:    badgesEl.length,
      badgeTexts:    Array.from(badgesEl).map((b) => b.textContent.trim().slice(0, 24)),
      // Colour / contrast probes.
      bgColor:       cs.backgroundColor,
      color:         cs.color,
      opacity:       cs.opacity,
      borderColor:   cs.borderColor,
      labelColor:    labelCs ? labelCs.color : '',
      labelOpacity:  labelCs ? labelCs.opacity : '',
      nameColor:     nameCs  ? nameCs.color : '',
      nameOpacity:   nameCs  ? nameCs.opacity : '',
    });
  });

  // ── Connectors ────────────────────────────────────────────────────
  const conns = $$('#gmap-svg path.gmap-connector');
  out.meta.connectorCount = conns.length;
  // Sample up to 80 connectors; usually ample for any single-service render.
  conns.slice(0, 80).forEach((p) => {
    const cs = getComputedStyle(p);
    out.connectors.push({
      src:    p.getAttribute('data-source-node-id') || '',
      dst:    p.getAttribute('data-target-node-id') || '',
      kind:   p.getAttribute('data-connector-kind') || '',
      cls:    p.getAttribute('class') || '',
      // Keep the path d truncated; the prefix is enough to identify start point.
      d:      (p.getAttribute('d') || '').slice(0, 280),
      stroke: cs.stroke,
      fill:   cs.fill,
      opacity: cs.opacity,
    });
  });

  // ── Selected node detail ──────────────────────────────────────────
  const sel = document.querySelector('.gmap-node.selected');
  if (sel) {
    const subs = [
      '.gmap-node-label',
      '.gmap-node-kind',
      '.gmap-node-name',
      '.gmap-node-title',
      '.gmap-node-meta',
      '.gmap-node-badge',
      '.gmap-node-actions',
      '.authority-badge',
      '[data-role="title"]',
    ];
    out.selected = {
      id:             sel.dataset.nodeId || '',
      kind:           sel.dataset.nodeKind || '',
      projectionKind: sel.dataset.projectionKind || '',
      cls:            sel.className || '',
      rect:           rectOf(sel),
      computedColor:  getComputedStyle(sel).color,
      computedBg:     getComputedStyle(sel).backgroundColor,
      computedBorder: getComputedStyle(sel).border,
      subElements:    subs.map((sub) => {
        const el = sel.querySelector(sub);
        if (!el) return { selector: sub, found: false };
        const cs = getComputedStyle(el);
        return {
          selector:   sub,
          found:      true,
          text:       (el.textContent || '').trim().slice(0, 80),
          color:      cs.color,
          background: cs.backgroundColor,
          opacity:    cs.opacity,
          visibility: cs.visibility,
          fontWeight: cs.fontWeight,
          fontSize:   cs.fontSize,
        };
      }),
    };
  }

  // ── Bottom rail / drift analytics intersection ───────────────────
  // Probe several likely selectors so the snippet is robust to the
  // exact markup the rail uses today.
  const railCandidates = [
    document.getElementById('drift-analytics'),
    document.querySelector('[data-rail="drift"]'),
    document.querySelector('aside[aria-label*="Drift" i]'),
    document.querySelector('.drift-analytics-rail'),
    document.querySelector('.drift-analytics'),
    document.querySelector('.governance-map-bottom-rail'),
    document.querySelector('.gmap-bottom-rail'),
    document.querySelector('.drift-rail'),
  ].filter(Boolean);
  if (railCandidates.length > 0) {
    const el = railCandidates[0];
    out.bottomRail = {
      tag:  el.tagName,
      id:   el.id || '',
      cls:  el.className || '',
      rect: rectOf(el),
    };
    out.meta.nodesClippedByBottomRail = out.nodes
      .filter((n) => n.rect.bottom > out.bottomRail.rect.top - 0)
      .map((n) => ({ id: n.id, bottom: n.rect.bottom, railTop: out.bottomRail.rect.top }));
  } else {
    out.notes.push('No bottom rail element matched the probed selectors.');
  }

  // Capture any other fixed/sticky element along the page bottom that
  // could be clipping the canvas, in case the rail uses a different name.
  $$('body *').slice(0, 0); // no-op to keep the body simple
  Array.from(document.body.children).forEach((el) => {
    const cs = getComputedStyle(el);
    if (cs.position === 'fixed' || cs.position === 'sticky') {
      const r = el.getBoundingClientRect();
      if (r.top >= window.innerHeight * 0.5 && r.height > 20) {
        out.bodyRails.push({
          tag: el.tagName,
          id:  el.id || '',
          cls: (el.className || '').slice(0, 80),
          rect: rectOf(el),
          position: cs.position,
        });
      }
    }
  });

  // ── Layer toggles ─────────────────────────────────────────────────
  $$('[data-authority-layer-chips] input, .authority-layer-chip, [data-layer-id]').forEach((el) => {
    const id = el.dataset.layerId || el.getAttribute('data-layer-id') || el.dataset.chip || '';
    if (!id) return;
    let checked;
    if (el.type === 'checkbox') checked = el.checked;
    else if (el.getAttribute('aria-pressed') !== null) checked = el.getAttribute('aria-pressed') === 'true';
    else checked = el.classList.contains('is-on') || el.classList.contains('is-active');
    out.layers[id] = checked;
  });

  // ── Authority layout spec cache (D32h-impl-1) ────────────────────
  try {
    const spec = window.MIDASExplorerGraph && window.MIDASExplorerGraph._lastAuthorityProjection;
    if (spec) {
      out.spec = {
        rootId:           spec.root && spec.root.id,
        chainCount:       (spec.chains || []).length,
        chainOrder:       (spec.chains || []).map((c) => c && c.chainId),
        chains:           (spec.chains || []).map((c) => ({
          chainId:        c.chainId,
          surfaceId:      c.surface && c.surface.id,
          profileId:      c.profile && c.profile.id,
          grantId:        c.grant   && c.grant.id,
          agentId:        c.agent   && c.agent.id,
          missingProfile: !!c.missingProfile,
          missingGrant:   !!c.missingGrant,
          missingAgent:   !!c.missingAgent,
          profileShared:  !!c.profileShared,
          grantShared:    !!c.grantShared,
          agentShared:    !!c.agentShared,
        })),
        governance:       {
          failModePolicies:  ((spec.governance || {}).failModePolicies  || []).map((g) => ({
            id:     g.node && g.node.id,
            owners: (g.owners || []).map((o) => ({ kind: o.kind, id: o.id })),
            shared: !!g.shared,
            bsDefault: !!g.bsDefault,
          })),
          escalationTargets: ((spec.governance || {}).escalationTargets || []).map((g) => ({
            id:     g.node && g.node.id,
            owners: (g.owners || []).map((o) => ({ kind: o.kind, id: o.id })),
            shared: !!g.shared,
          })),
        },
        unlinkedIds:      (spec.unlinked || []).map((n) => n && n.id),
      };
    } else {
      out.notes.push('No cached spec on window.MIDASExplorerGraph._lastAuthorityProjection — lens may not be Authority.');
    }
  } catch (e) {
    out.notes.push('Spec read failed: ' + (e && e.message));
  }

  // ── Output ────────────────────────────────────────────────────────
  const json = JSON.stringify(out, null, 2);
  console.log('[D32h-fix-1] snapshot (nodes=%d, connectors=%d, lens=%s):',
    out.meta.nodeCount, out.meta.connectorCount, out.meta.activeLens);
  console.log(json);
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(json)
      .then(() => console.log('[D32h-fix-1] Snapshot copied to clipboard. Paste into chat.'))
      .catch(() => console.warn('[D32h-fix-1] Could not copy to clipboard (focus the page first).'));
  }
  return out;
})();
