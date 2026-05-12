// /explorer/assets/js/records/audit-event-renderers.js — D29g + D29l
//
// Audit-event renderers for the Records detail rail. Populates
// window.MIDASExplorerRecords.auditEventRenderers with:
//
//   - renderAuditEventCard(ev)                 — dispatch helper
//   - renderFailModePolicyTriggerFired(ev)     — rich D29l renderer
//                                                for FAIL_MODE_POLICY_TRIGGER_FIRED
//   - renderFailModePolicyDryRunDecision(ev)   — rich D29l renderer
//                                                for FAIL_MODE_POLICY_DRY_RUN_DECISION
//   - renderFailModePolicyEnforced(ev)         — rich D29g renderer for
//                                                FAIL_MODE_POLICY_ENFORCED
//                                                (trigger-aware after D29l)
//   - renderGenericAuditEvent(ev)              — minimal fallback for
//                                                every other event kind
//
// Read-only by design. The dispatch helper never calls a mutating
// HTTP method. Every rich renderer reads the payload that the runtime
// emits verbatim — it does not reshape field names or invent values.
// Approach (A) from D29g Step 0 section 5 is used for tension copy:
// previous-path / enforced-path language describes the delta in
// outcome-only terms without naming the authority profile's fail-mode
// setting.
//
// D29l adds trigger-aware copy for the authority_resolution_failure
// trigger condition. On that path the orchestrator short-circuits
// before reaching the evaluator step, so authority resolution
// failed before policy evaluation ever ran. The authority copy
// surfaces that ordering precisely and never asserts the evaluator
// itself produced an error — because it was never invoked.
//
// File location convention: parallels envelope-summary.js and
// evidence-helpers.js. evidence-helpers.js remains an empty namespace
// stub so the D27j-ui-foundation-6 forbidden-pin tests stay green.
//
// Operator-facing copy intentionally describes deltas only in terms
// of the previous / enforced outcome words — never naming the
// underlying authority fail-mode setting. This is the approach
// agreed for D29g: keep the explanation about WHAT changed, not
// about which configuration produced the change.

(function () {
  'use strict';

  window.MIDASExplorerRecords = window.MIDASExplorerRecords || {};

  const Utils = window.MIDASExplorerUtils || {};
  // escHtml is exposed by util-format.js (D27j-ui-foundation-3).
  // Fallback to a local escape so this module degrades gracefully
  // if loaded out of order — but the production load order
  // guarantees util-format.js runs first.
  const escHtml = (typeof Utils.escHtml === 'function')
    ? Utils.escHtml
    : function (s) {
        return String(s == null ? '' : s)
          .replace(/&/g, '&amp;')
          .replace(/</g, '&lt;')
          .replace(/>/g, '&gt;')
          .replace(/"/g, '&quot;')
          .replace(/'/g, '&#39;');
      };

  // outcomeWord maps an eval.Outcome string to the natural-language
  // word used in tension copy. Matches the existing Explorer
  // convention from envelope-summary.js which accepts both 'accept'
  // and 'approve' literals. The mapping below converts the wire
  // outcome strings to past-tense verbs that read naturally in the
  // four tension copy variants.
  function outcomeWord(outcome) {
    const o = String(outcome || '').toLowerCase();
    if (o === 'accept' || o === 'approve' || o === 'permit_with_evidence') {
      return 'permitted';
    }
    if (o === 'reject') return 'rejected';
    if (o === 'escalate') return 'escalated';
    if (o === 'request_clarification' || o === 'clarify') return 'requested clarification';
    return o || '—';
  }

  // outcomeIsPermissive returns true for outcomes that proceed
  // (accept / permit / permit_with_evidence). Used to classify the
  // four tension-copy cases without naming the authority profile's fail-mode setting.
  function outcomeIsPermissive(outcome) {
    const o = String(outcome || '').toLowerCase();
    return o === 'accept' || o === 'approve' || o === 'permit_with_evidence';
  }

  // tensionCopy returns the natural-language explanation of the
  // delta between previous_outcome and enforced_outcome. Covers the
  // four operator-surprise cases from D29g §4. When the previous and
  // enforced outcomes do not match any of the four named shapes, the
  // helper returns '' so the renderer can omit the tension paragraph
  // rather than emit copy that does not describe the actual delta.
  function tensionCopy(previousOutcome, enforcedOutcome) {
    const prev = String(previousOutcome || '').toLowerCase();
    const enf = String(enforcedOutcome || '').toLowerCase();
    if (prev === 'escalate' && outcomeIsPermissive(enf)) {
      return 'FailModePolicy permitted execution where the previous path would have escalated.';
    }
    if (outcomeIsPermissive(prev) && enf === 'reject') {
      return 'FailModePolicy rejected execution where the previous path would have proceeded.';
    }
    if (outcomeIsPermissive(prev) && enf === 'escalate') {
      return 'FailModePolicy escalated execution where the previous path would have proceeded.';
    }
    if (prev === 'escalate' && enf === 'reject') {
      return 'FailModePolicy rejected execution where the previous path would have escalated.';
    }
    return '';
  }

  // deltaClass returns the CSS marker class describing the
  // relationship between previous_* and enforced_* values. Exactly
  // one of the three values is returned per emission.
  function deltaClass(prevOutcome, enfOutcome, prevReason, enfReason) {
    if (prevOutcome !== enfOutcome) return 'is-changed';
    if (prevReason !== enfReason) return 'is-same-outcome';
    return 'is-identical';
  }

  // deltaLeadCopy returns the human-readable summary of the delta
  // category. Matches the three CSS markers one-for-one.
  function deltaLeadCopy(cls) {
    if (cls === 'is-changed') {
      return 'Enforcement changed the runtime outcome.';
    }
    if (cls === 'is-same-outcome') {
      return 'Enforcement preserved the runtime outcome but changed the recorded reason.';
    }
    return 'Enforcement was applied; runtime outcome and reason matched the previous path.';
  }

  // isAuthorityResolutionFailureTrigger returns true when the supplied
  // trigger condition is the D29j authority_resolution_failure kind.
  // Used by D29l to gate authority-specific copy without leaking the
  // string into multiple call sites.
  function isAuthorityResolutionFailureTrigger(triggerCondition) {
    return String(triggerCondition || '') === 'authority_resolution_failure';
  }

  // authorityFailureCauseCopy maps the authority-chain reason code
  // recorded on the audit payload (previous_reason_code on enforced /
  // dry-run events, or absent on trigger-fired) to a single-sentence
  // operator-readable cause string. Unknown / empty reason codes fall
  // back to the generic line.
  //
  // Pure function. Exposed for tests; no DOM access, no payload
  // mutation. Returns plain text; the caller is responsible for
  // HTML-escaping when interpolating into markup.
  function authorityFailureCauseCopy(previousReasonCode) {
    const code = String(previousReasonCode || '').toUpperCase();
    if (code === 'NO_ACTIVE_GRANT') {
      return 'No active authority grant was available.';
    }
    if (code === 'PROFILE_NOT_FOUND') {
      return 'No active authority profile could be resolved.';
    }
    if (code === 'GRANT_PROFILE_SURFACE_MISMATCH') {
      return 'The resolved authority profile did not match the decision surface.';
    }
    return 'Authority resolution failed before policy evaluation.';
  }

  // authorityResolutionFailureLeadCopy returns the per-event-kind
  // authority-specific note. Each variant is precise about what
  // happened: authority resolution failed before the evaluator step
  // was reached. The wording never claims the evaluator itself
  // produced an error — that would falsely imply the evaluator
  // started.
  //
  // The third argument is the delta class for the enforced renderer
  // ('is-changed' / 'is-same-outcome' / 'is-identical'); other
  // renderers pass undefined.
  function authorityResolutionFailureLeadCopy(eventKind, deltaCls) {
    if (eventKind === 'FAIL_MODE_POLICY_TRIGGER_FIRED') {
      return 'Authority resolution failed before policy evaluation.';
    }
    if (eventKind === 'FAIL_MODE_POLICY_DRY_RUN_DECISION') {
      return 'Authority resolution failed before policy evaluation; the dry-run result was recorded without changing the runtime outcome.';
    }
    if (eventKind === 'FAIL_MODE_POLICY_ENFORCED') {
      if (deltaCls === 'is-identical') {
        return 'Authority resolution failed before policy evaluation; FailModePolicy enforcement preserved the runtime outcome.';
      }
      return 'Authority resolution failed before policy evaluation; FailModePolicy enforcement changed how the authority failure was handled.';
    }
    return 'Authority resolution failed before policy evaluation.';
  }

  // kvRow renders a single key/value row inside a renderer card.
  // Both key and value are HTML-escaped. The renderer uses sentence
  // case for the key label per the MIDAS editorial preference.
  function kvRow(label, value) {
    const v = (value === undefined || value === null || value === '') ? '—' : String(value);
    return (
      '<div class="failmode-enforcement-kv">' +
        '<span class="failmode-enforcement-kv-key">' + escHtml(label) + '</span>' +
        '<span class="failmode-enforcement-kv-value">' + escHtml(v) + '</span>' +
      '</div>'
    );
  }

  // kvRowMono renders a key/value row whose value is mono-spaced
  // (policy ids, hashes, RFC3339 timestamps). The value carries
  // .failmode-enforcement-code so it can be styled distinctly.
  function kvRowMono(label, value) {
    const v = (value === undefined || value === null || value === '') ? '—' : String(value);
    return (
      '<div class="failmode-enforcement-kv">' +
        '<span class="failmode-enforcement-kv-key">' + escHtml(label) + '</span>' +
        '<span class="failmode-enforcement-kv-value">' +
          '<code class="failmode-enforcement-code">' + escHtml(v) + '</code>' +
        '</span>' +
      '</div>'
    );
  }

  // renderFailModePolicyEnforced is the rich D29g renderer for
  // FAIL_MODE_POLICY_ENFORCED audit events. Reads the payload that
  // D29f emits verbatim — see internal/decision/orchestrator.go's
  // appendFailModePolicyEnforcedEvent for the field list.
  //
  // The renderer is HTML-only: it constructs a string of escaped
  // markup and returns it. The caller writes the string into the
  // DOM (Records detail rail). No fetch, no DOM mutation outside
  // the returned string. Read-only by construction.
  function renderFailModePolicyEnforced(ev) {
    if (!ev || typeof ev !== 'object') return '';
    const p = (ev.payload && typeof ev.payload === 'object') ? ev.payload : {};

    const policyId         = String(p.fail_mode_policy_id      || '');
    const policyVersion    = (typeof p.fail_mode_policy_version === 'number')
                              ? String(p.fail_mode_policy_version) : '';
    const source           = String(p.source                    || '');
    const trigger          = String(p.trigger_condition         || '');
    const correctnessClass = String(p.correctness_class         || '');
    const permittedMode    = String(p.permitted_mode            || '');
    const enforcementState = String(p.enforcement_state         || '');
    const configuredOutcome= String(p.configured_outcome        || '');
    const enforcedOutcome  = String(p.enforced_outcome          || '');
    const enforcedReason   = String(p.enforced_reason_code      || '');
    const previousOutcome  = String(p.previous_outcome          || '');
    const previousReason   = String(p.previous_reason_code      || '');
    const appliedAt        = String(p.applied_at                || '');

    const policyLabel = policyVersion
      ? policyId + ' v' + policyVersion
      : policyId;

    const primaryRows =
      kvRowMono('Policy', policyLabel) +
      kvRow('Source', source) +
      kvRow('Trigger', trigger) +
      kvRow('Correctness class', correctnessClass) +
      kvRow('Configured outcome', configuredOutcome) +
      kvRow('Previous', previousOutcome + ' / ' + previousReason) +
      kvRow('Enforced', enforcedOutcome + ' / ' + enforcedReason) +
      kvRowMono('Applied at', appliedAt);

    // Delta block — exactly one of three CSS markers, plus its
    // matching lead copy. Reason-code comparison applies only when
    // outcomes match (same-outcome vs identical); when outcomes
    // differ the delta is unambiguously is-changed regardless of
    // reason-code values.
    const cls = deltaClass(previousOutcome, enforcedOutcome, previousReason, enforcedReason);
    const lead = deltaLeadCopy(cls);
    const deltaDetail =
      '<div class="failmode-enforcement-delta-detail">' +
        '<span class="failmode-enforcement-delta-pair">' +
          '<span class="failmode-enforcement-delta-label">Previous outcome</span>' +
          '<code class="failmode-enforcement-code">' + escHtml(previousOutcome || '—') + ' / ' + escHtml(previousReason || '—') + '</code>' +
        '</span>' +
        '<span class="failmode-enforcement-delta-pair">' +
          '<span class="failmode-enforcement-delta-label">Enforced outcome</span>' +
          '<code class="failmode-enforcement-code">' + escHtml(enforcedOutcome || '—') + ' / ' + escHtml(enforcedReason || '—') + '</code>' +
        '</span>' +
      '</div>';
    const deltaBlock =
      '<div class="failmode-enforcement-delta ' + cls + '">' +
        '<p class="failmode-enforcement-delta-lead">' + escHtml(lead) + '</p>' +
        deltaDetail +
      '</div>';

    // Tension copy paragraph — describes the delta in outcome words.
    // Empty for combinations that do not match the four named cases
    // (e.g. is-identical, where there is no delta to explain).
    const tension = tensionCopy(previousOutcome, enforcedOutcome);
    const tensionBlock = tension
      ? '<p class="failmode-enforcement-tension">' + escHtml(tension) + '</p>'
      : '';

    // D29l authority-resolution note — rendered only when the trigger
    // condition is authority_resolution_failure. Explains that
    // authority resolution failed BEFORE the evaluator step was
    // reached (so the operator does not infer the evaluator ran) and
    // names the underlying authority-chain cause from
    // previous_reason_code. Identical / changed wording is selected
    // from the delta class.
    let authorityBlock = '';
    if (isAuthorityResolutionFailureTrigger(trigger)) {
      const note = authorityResolutionFailureLeadCopy('FAIL_MODE_POLICY_ENFORCED', cls);
      const cause = authorityFailureCauseCopy(previousReason);
      authorityBlock =
        '<div class="failmode-authority-note">' +
          '<p class="failmode-authority-note-lead">' + escHtml(note) + '</p>' +
          '<p class="failmode-authority-cause">' + escHtml(cause) + '</p>' +
        '</div>';
    }

    // Secondary contextual fields — rendered only when present on
    // the payload. The contextual block is omitted entirely when no
    // contextual fields are populated, keeping the card compact.
    const surfaceId         = String(p.surface_id            || '');
    const surfaceVersion    = (typeof p.surface_version === 'number')
                                ? String(p.surface_version) : '';
    const businessServiceId = String(p.business_service_id   || '');
    const authorityProfileId= String(p.authority_profile_id  || '');
    const agentId           = String(p.agent_id              || '');
    const policyReference   = String(p.policy_reference      || '');

    let contextRows = '';
    if (surfaceId) {
      const surfaceLabel = surfaceVersion ? surfaceId + ' v' + surfaceVersion : surfaceId;
      contextRows += kvRowMono('Surface', surfaceLabel);
    }
    if (businessServiceId) {
      contextRows += kvRowMono('Business service', businessServiceId);
    }
    if (authorityProfileId) {
      contextRows += kvRowMono('Authority profile', authorityProfileId);
    }
    if (agentId) {
      contextRows += kvRowMono('Agent', agentId);
    }
    if (policyReference) {
      contextRows += kvRow('Policy reference', policyReference);
    }
    const contextBlock = contextRows
      ? '<div class="failmode-enforcement-context">' +
          '<p class="failmode-enforcement-section-label">Context</p>' +
          contextRows +
        '</div>'
      : '';

    // Hidden trigger-condition / posture details are kept in the
    // primary rows above; the secondary section is reserved for
    // identity-shaped fields the operator may want to jump to.
    // permitted_mode and enforcement_state are not in the primary
    // grid because the renderer's CSS marker already encodes the
    // delta state visually — including them as kv rows would clutter
    // the card without adding decision-relevant context. They remain
    // available on the underlying audit event payload.
    void permittedMode;
    void enforcementState;

    return (
      '<article class="failmode-enforcement-card" data-event-type="FAIL_MODE_POLICY_ENFORCED">' +
        '<header class="failmode-enforcement-card-header">' +
          '<span class="failmode-enforcement-badge">FailModePolicy enforced</span>' +
        '</header>' +
        '<div class="failmode-enforcement-primary">' +
          primaryRows +
        '</div>' +
        deltaBlock +
        tensionBlock +
        authorityBlock +
        contextBlock +
      '</article>'
    );
  }

  // renderFailModePolicyTriggerFired is the D29l rich renderer for
  // FAIL_MODE_POLICY_TRIGGER_FIRED audit events. Reads the payload
  // emitted by appendFailModePolicyTriggerFiredEvent verbatim. The
  // trigger-fired event is evidence-only — it does not branch the
  // runtime outcome. The renderer therefore omits the previous /
  // enforced delta block and surfaces trigger / rule / policy
  // identity instead.
  //
  // When trigger_condition is authority_resolution_failure, an
  // authority-specific note is appended below the primary rows so
  // operators see that authority resolution failed before policy
  // evaluation ran. When rule_status is "not_found" the defensive
  // marker is rendered so operators are not left wondering why the
  // rule fields are blank.
  //
  // HTML-only. Read-only by construction.
  function renderFailModePolicyTriggerFired(ev) {
    if (!ev || typeof ev !== 'object') return '';
    const p = (ev.payload && typeof ev.payload === 'object') ? ev.payload : {};

    const policyId         = String(p.fail_mode_policy_id      || '');
    const policyVersion    = (typeof p.fail_mode_policy_version === 'number')
                              ? String(p.fail_mode_policy_version) : '';
    const source           = String(p.source                    || '');
    const trigger          = String(p.trigger_condition         || '');
    const correctnessClass = String(p.correctness_class         || '');
    const permittedMode    = String(p.permitted_mode            || '');
    const enforcementState = String(p.enforcement_state         || '');
    const outcome          = String(p.outcome                   || '');
    const triggeredAt      = String(p.triggered_at              || '');
    const evaluationTime   = String(p.evaluation_time           || '');
    const ruleStatus       = String(p.rule_status               || '');

    const policyLabel = policyVersion
      ? policyId + ' v' + policyVersion
      : policyId;
    const ruleLabel = (permittedMode || enforcementState || outcome)
      ? [permittedMode || '—', enforcementState || '—', outcome || '—'].join(' / ')
      : '—';

    const primaryRows =
      kvRow('Trigger', trigger) +
      kvRow('Correctness class', correctnessClass) +
      kvRowMono('Policy', policyLabel) +
      kvRow('Source', source) +
      kvRow('Configured rule', ruleLabel) +
      kvRowMono('Triggered at', triggeredAt) +
      kvRowMono('Evaluation time', evaluationTime);

    let authorityBlock = '';
    if (isAuthorityResolutionFailureTrigger(trigger)) {
      const note = authorityResolutionFailureLeadCopy('FAIL_MODE_POLICY_TRIGGER_FIRED');
      authorityBlock =
        '<div class="failmode-authority-note">' +
          '<p class="failmode-authority-note-lead">' + escHtml(note) + '</p>' +
        '</div>';
    }

    let ruleStatusBlock = '';
    if (ruleStatus === 'not_found') {
      ruleStatusBlock =
        '<p class="failmode-trigger-rule-status">' +
          escHtml('No matching FailModePolicy rule was found for the trigger correctness class.') +
        '</p>';
    }

    // Secondary contextual fields — only render those present. On
    // authority_resolution_failure paths authority_profile_id and
    // policy_reference are absent because the profile was not
    // resolved; the conditional renders no Authority profile / Policy
    // reference row in that case.
    const surfaceId         = String(p.surface_id            || '');
    const surfaceVersion    = (typeof p.surface_version === 'number')
                                ? String(p.surface_version) : '';
    const businessServiceId = String(p.business_service_id   || '');
    const authorityProfileId= String(p.authority_profile_id  || '');
    const agentId           = String(p.agent_id              || '');
    const policyReference   = String(p.policy_reference      || '');

    let contextRows = '';
    if (surfaceId) {
      const surfaceLabel = surfaceVersion ? surfaceId + ' v' + surfaceVersion : surfaceId;
      contextRows += kvRowMono('Surface', surfaceLabel);
    }
    if (businessServiceId) {
      contextRows += kvRowMono('Business service', businessServiceId);
    }
    if (authorityProfileId) {
      contextRows += kvRowMono('Authority profile', authorityProfileId);
    }
    if (agentId) {
      contextRows += kvRowMono('Agent', agentId);
    }
    if (policyReference) {
      contextRows += kvRow('Policy reference', policyReference);
    }
    const contextBlock = contextRows
      ? '<div class="failmode-enforcement-context">' +
          '<p class="failmode-enforcement-section-label">Context</p>' +
          contextRows +
        '</div>'
      : '';

    return (
      '<article class="failmode-trigger-card" data-event-type="FAIL_MODE_POLICY_TRIGGER_FIRED">' +
        '<header class="failmode-trigger-card-header">' +
          '<span class="failmode-enforcement-badge">FailModePolicy trigger fired</span>' +
        '</header>' +
        '<div class="failmode-enforcement-primary">' +
          primaryRows +
        '</div>' +
        authorityBlock +
        ruleStatusBlock +
        contextBlock +
      '</article>'
    );
  }

  // renderFailModePolicyDryRunDecision is the D29l rich renderer for
  // FAIL_MODE_POLICY_DRY_RUN_DECISION audit events. Reads the payload
  // emitted by appendFailModePolicyDryRunDecisionEvent verbatim.
  //
  // The dry-run event records the would-be outcome alongside the
  // actual outcome the runtime applied. The renderer renders both
  // pairs side-by-side with a divergent marker. When the trigger
  // condition is authority_resolution_failure, an authority-specific
  // note clarifies that authority resolution failed before policy
  // evaluation ran and that the dry-run did not change the runtime
  // outcome.
  //
  // HTML-only. Read-only by construction.
  function renderFailModePolicyDryRunDecision(ev) {
    if (!ev || typeof ev !== 'object') return '';
    const p = (ev.payload && typeof ev.payload === 'object') ? ev.payload : {};

    const policyId           = String(p.fail_mode_policy_id      || '');
    const policyVersion      = (typeof p.fail_mode_policy_version === 'number')
                                 ? String(p.fail_mode_policy_version) : '';
    const source             = String(p.source                    || '');
    const trigger            = String(p.trigger_condition         || '');
    const correctnessClass   = String(p.correctness_class         || '');
    const configuredOutcome  = String(p.configured_outcome        || '');
    const actualOutcome      = String(p.actual_outcome            || '');
    const actualReason       = String(p.actual_reason_code        || '');
    const dryRunOutcome      = String(p.dry_run_outcome           || '');
    const dryRunReason       = String(p.dry_run_reason_code       || '');
    const divergent          = (p.divergent === true) ? 'true' : 'false';
    const computedAt         = String(p.computed_at               || '');
    const evaluationTime     = String(p.evaluation_time           || '');

    const policyLabel = policyVersion
      ? policyId + ' v' + policyVersion
      : policyId;
    const actualPair = (actualOutcome || actualReason)
      ? (actualOutcome || '—') + ' / ' + (actualReason || '—')
      : '—';
    const dryRunPair = (dryRunOutcome || dryRunReason)
      ? (dryRunOutcome || '—') + ' / ' + (dryRunReason || '—')
      : '—';

    const primaryRows =
      kvRow('Trigger', trigger) +
      kvRow('Correctness class', correctnessClass) +
      kvRowMono('Policy', policyLabel) +
      kvRow('Source', source) +
      kvRow('Configured outcome', configuredOutcome) +
      kvRow('Actual outcome', actualPair) +
      kvRow('Dry-run outcome', dryRunPair) +
      kvRow('Divergent', divergent) +
      kvRowMono('Computed at', computedAt) +
      kvRowMono('Evaluation time', evaluationTime);

    let authorityBlock = '';
    if (isAuthorityResolutionFailureTrigger(trigger)) {
      const note = authorityResolutionFailureLeadCopy('FAIL_MODE_POLICY_DRY_RUN_DECISION');
      const cause = authorityFailureCauseCopy(actualReason);
      authorityBlock =
        '<div class="failmode-authority-note">' +
          '<p class="failmode-authority-note-lead">' + escHtml(note) + '</p>' +
          '<p class="failmode-authority-cause">' + escHtml(cause) + '</p>' +
        '</div>';
    }

    const surfaceId         = String(p.surface_id            || '');
    const surfaceVersion    = (typeof p.surface_version === 'number')
                                ? String(p.surface_version) : '';
    const businessServiceId = String(p.business_service_id   || '');
    const authorityProfileId= String(p.authority_profile_id  || '');
    const agentId           = String(p.agent_id              || '');
    const policyReference   = String(p.policy_reference      || '');

    let contextRows = '';
    if (surfaceId) {
      const surfaceLabel = surfaceVersion ? surfaceId + ' v' + surfaceVersion : surfaceId;
      contextRows += kvRowMono('Surface', surfaceLabel);
    }
    if (businessServiceId) {
      contextRows += kvRowMono('Business service', businessServiceId);
    }
    if (authorityProfileId) {
      contextRows += kvRowMono('Authority profile', authorityProfileId);
    }
    if (agentId) {
      contextRows += kvRowMono('Agent', agentId);
    }
    if (policyReference) {
      contextRows += kvRow('Policy reference', policyReference);
    }
    const contextBlock = contextRows
      ? '<div class="failmode-enforcement-context">' +
          '<p class="failmode-enforcement-section-label">Context</p>' +
          contextRows +
        '</div>'
      : '';

    return (
      '<article class="failmode-dryrun-card" data-event-type="FAIL_MODE_POLICY_DRY_RUN_DECISION">' +
        '<header class="failmode-enforcement-card-header">' +
          '<span class="failmode-enforcement-badge">FailModePolicy dry-run decision</span>' +
        '</header>' +
        '<div class="failmode-enforcement-primary">' +
          primaryRows +
        '</div>' +
        authorityBlock +
        contextBlock +
      '</article>'
    );
  }

  // renderGenericAuditEvent is the minimal fallback for audit event
  // kinds without a rich renderer. Shows event_type, occurred_at
  // (when present), and a single-line payload preview. Falls back
  // to "(no payload)" when the payload is empty.
  function renderGenericAuditEvent(ev) {
    if (!ev || typeof ev !== 'object') return '';
    const type = String(ev.event_type || '');
    const occ = String(ev.occurred_at || '');
    return (
      '<article class="failmode-enforcement-card audit-event-card-generic" data-event-type="' + escHtml(type) + '">' +
        '<header class="failmode-enforcement-card-header">' +
          '<span class="failmode-enforcement-badge audit-event-badge-generic">' + escHtml(type) + '</span>' +
        '</header>' +
        '<div class="failmode-enforcement-primary">' +
          kvRowMono('Occurred at', occ) +
        '</div>' +
      '</article>'
    );
  }

  // renderAuditEventCard dispatches on event_type and returns the
  // rendered HTML string. Unknown event kinds fall through to the
  // generic renderer so the audit list always renders something
  // meaningful, even for event kinds future tranches add without
  // dedicated renderers.
  function renderAuditEventCard(ev) {
    if (!ev || typeof ev !== 'object') return '';
    const type = String(ev.event_type || '');
    if (type === 'FAIL_MODE_POLICY_ENFORCED') {
      return renderFailModePolicyEnforced(ev);
    }
    if (type === 'FAIL_MODE_POLICY_TRIGGER_FIRED') {
      return renderFailModePolicyTriggerFired(ev);
    }
    if (type === 'FAIL_MODE_POLICY_DRY_RUN_DECISION') {
      return renderFailModePolicyDryRunDecision(ev);
    }
    return renderGenericAuditEvent(ev);
  }

  window.MIDASExplorerRecords.auditEventRenderers = {
    renderAuditEventCard:               renderAuditEventCard,
    renderFailModePolicyEnforced:       renderFailModePolicyEnforced,
    renderFailModePolicyTriggerFired:   renderFailModePolicyTriggerFired,
    renderFailModePolicyDryRunDecision: renderFailModePolicyDryRunDecision,
    renderGenericAuditEvent:            renderGenericAuditEvent,
    // Exposed for tests; pure functions with no side-effects.
    outcomeWord:                                outcomeWord,
    outcomeIsPermissive:                        outcomeIsPermissive,
    tensionCopy:                                tensionCopy,
    deltaClass:                                 deltaClass,
    deltaLeadCopy:                              deltaLeadCopy,
    authorityFailureCauseCopy:                  authorityFailureCauseCopy,
    authorityResolutionFailureLeadCopy:         authorityResolutionFailureLeadCopy,
    isAuthorityResolutionFailureTrigger:        isAuthorityResolutionFailureTrigger,
  };
})();
