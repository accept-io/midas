# Agentic AI Behavioural Drift — Consolidated Briefing for MIDAS (v5)

*Builds on v3 (which applied the calibration critique: drift separated from cause, softened mechanism/consensus claims, re-attributed NIST, scoped the single-spike rule, fixed the logging/privacy nuance). v4 folds in the production-monitoring additions from the latest draft — label latency and label-free estimation, release-time canary/shadow defences, internal-state and model-performance detection families, and a consolidated KPI set — while restoring the standards mapping, goal-conditioned baselining, and first-class authority-path detection that the latest draft dropped.*

**Changes from v4:**
1. Renamed the closing section to "Open decisions and calibration notes" — several items are now resolved and recorded as decisions, not open conflicts.
2. Resolved the composite-score stance: a per-surface triage and prioritisation signal, not the primary paging condition.
3. Clarified the agency-risk index as a future design primitive — a qualitative tier first, formalised into a scored index once enough telemetry exists.

**Changes from v3:**
1. Added **label latency / delayed-or-absent ground truth** as a first-class pitfall, with **confidence-based (label-free) performance estimation** and sampled human review as the response.
2. Added **release-time defences** — canary, shadow, and mirrored/blue-green traffic — as both a detection family and a release gate.
3. Added **internal-state/activation** and **model-performance** monitoring to the detection families, with an explicit caveat that activation monitors require model internals and likely exceed MIDAS's governance/inspection-layer access.
4. Added a consolidated **MIDAS KPI set** and a short **implementation-pitfalls** block.
5. **Retained/restored** (the latest draft had dropped these): the OWASP/MITRE/ISO standards mapping, goal-conditioned baselining plus the benign-vs-harmful framing, and first-class authority-path detection.

## Executive summary

There is still no standards-grade definition of "agentic drift." The defensible working definition is: **a statistically or behaviourally significant divergence over time between an agentic system's expected and observed behaviour, decisions, trajectories, or outcomes under real operating conditions.** Causes — input/data shift, environment change, tool changes, prompt/policy edits, model updates, memory/context effects, or reward/preference tuning — are attributed *separately*, not folded into the definition, because runtime drift, release regressions, and configuration changes call for different responses. This is broader than classic ML drift because agentic systems do not merely predict — they plan, call tools, maintain context, and execute multi-step trajectories.

Drift often begins before any explicit policy breach, so monitoring should watch **weak signals** — guardrail-rate changes, trajectory anomalies, reviewer feedback, user complaints — rather than wait for a rule to trip. It does not, by nature, evade monitoring; it evades *naive rule-based* guardrails because no rule is broken at onset.

Detection is a layered, ensemble problem, not a single score: distribution statistics on inputs/outputs; online change detectors for low-latency alerting; behavioural and trajectory evaluation of planning, tool use, and completion; internal-state monitors where feasible; release-time canary/shadow checks; and post-alert diagnosis through attribution and counterfactuals.

For MIDAS, four nested units of analysis carry the design — **these are MIDAS product-design choices, not externally standardised units** (agent-evaluation research is explicitly multi-perspectival, and "decision surface" is not a field-standard term; the field does, however, consistently favour monitoring at the smallest actionable unit and rolling up for governance):

- **Decision surface = MIDAS's chosen primary alert key.** The narrowest governed point where goals, context, tools, authority, and consequences meet; alerts are raised here.
- **Authority profile = the primary explanatory lens for high-severity alerts.** Authority/permission-path divergence is a first-class drift class (MIDAS's differentiator) and explains the severe cases.
- **Agent = monitored for route and execution quality** (latency, retries, tool mix, memory growth, version drift).
- **Capability = the roll-up view** for prioritisation and governance, not the paging signal.

The third design position, retained because the standards layer keeps getting dropped from drafts: **map MIDAS's drift model explicitly onto the agentic-security standards** (OWASP Agentic Top 10, NIST AI RMF + GenAI Profile, MITRE ATLAS, ISO/IEC 42001). For a CNCF-submitted governance tool, that mapping is much of what confers external credibility.

## 1. What agentic drift is

**Working definition (shared).** Agentic drift is the observed, often gradual divergence of an autonomous agent's behaviour, goals, tool-use, decision path, or authority from its intended or originally-specified behaviour, measured at runtime. Keep the observed divergence distinct from its hypothesised cause: the field already suffers terminology confusion, and mitigation depends on identifying the specific shift type and root cause, so detecting a shift is not sufficient.

**Synthesis of three accepted ideas:**
- **Distribution shift** — inputs, outputs, or labels no longer follow the reference distribution (Hinder et al. on concept drift);
- **LLM behaviour drift** — the effective behaviour of the "same" model service changes across updates or operating conditions (Chen, Zaharia, Zou);
- **Agent task/trajectory drift** — the system still returns an answer, but its plan, tool use, or decision path increasingly diverges from the intended path (Abdelnabi et al.'s "task drift").

Agentic systems fail in ways ordinary prediction systems do not: agent-evaluation surveys treat planning, tool use, self-reflection, memory, multi-turn interaction, and dynamic-environment handling as first-class evaluation dimensions, and long-horizon work shows failure can be a *trajectory reliability* problem — the agent can solve the task yet drifts off a canonical solution path as the run unfolds.

**Statistical taxonomy (base layer).** From classic ML: **covariate drift** changes P(X); **label drift** changes P(Y); **concept drift** changes P(Y∣X) or the effective decision boundary. Agentic systems add layers — these are *causes/types to attribute*, not part of the definition:

| Drift sub-type | What diverges | Signal class (per decision surface) |
|---|---|---|
| Data / context drift | Inputs P(X), retrieved material, environment observations | Context |
| Output / performance drift | Output distributions, task success, calibration | Outcome |
| Trajectory / route drift | Plan, tool sequence, handoff pattern | Trajectory |
| Coordination drift | Multi-agent consensus, handoffs, role adherence | Trajectory (multi-agent) |
| Reward / utility drift | Optimisation/feedback weighting (e.g., GPT-4o) | Risk/control |
| Policy / specification / safety drift | Adherence to prompts, guardrails, policy packs | Risk/control |
| Persona / interaction drift | Sycophancy, role-inconsistency, behavioural instability over turns | Outcome + trajectory |
| **Authority drift (permission creep)** | Effective permission path / delegated authority vs policy | **Risk/control — first-class for MIDAS** |
| Data-pipeline drift | Schema, null rate, type, out-of-bounds | Context / system |
| System-health drift | Latency, token use, cost, error rate | System |

**Canonical real-world example.** OpenAI's April 2025 GPT-4o rollback is an official example of reward/utility drift: the update became overly flattering/agreeable because it over-weighted short-term feedback without accounting for how interactions evolve, and was rolled back. Framing note: this illustrates the *LLM-behaviour-drift-across-updates* leg — a discrete release caught and reverted, closer to a release regression than slow silent divergence — and shows behaviour change *can* be surfaced and remediated. Use it as the across-update case, not the silent-drift case.

**Mechanism.** Long conversations can degrade behavioural consistency, but the causal story is not settled. Persona/identity drift appears to arise from several interacting factors — long-context positional bias ("lost in the middle"), post-training persona dynamics, and conversation structure (meta-reflective dialogues as triggers) — rather than a single mechanism. Treat the existence of persona drift as well-supported and its precise mechanism as open.

**Scope note.** Drift is not the same as degradation, and neither is identical to attack: a system can drift silently before quality falls; it can degrade with no obvious distribution change; and an attack can masquerade as drift. Model drift as "observed vs expected" at the decision-surface level, then explain by entity, cohort, version, and authority path. (Whether MIDAS *claims to detect* underlying model/concept drift or only *attributes* to it remains a product decision — see Open decisions §3.)

## 2. Standards and governance mapping

Retained because it matters disproportionately for a governance product and keeps getting dropped from drafts.

- **OWASP Top 10 for Agentic Applications** (Agentic Security Initiative, Dec 2025). Direct matches: **ASI10 Rogue Agents** (behavioural drift, collusion, concealment, self-directed action by drifting/misaligned agents); **ASI01 Agent Goal Hijack** (goal/semantic drift under manipulation); **ASI03 Identity & Privilege Abuse** (authority/permission creep); **ASI09 Human-Agent Trust Exploitation** (authority-bias manipulation). The "**Least Agency**" principle (autonomy earned, not default) is a natural organising idea for the Authority lens. Companion method: **MAESTRO** threat-modelling.
- **NIST AI RMF 1.0** (GOVERN / MAP / MEASURE / MANAGE) is explicitly lifecycle-wide and names operation and monitoring as lifecycle tasks; risk tolerance and metric thresholds are organisation- and use-case-specific, set with context and human judgment. The **NIST GenAI Profile** specifies post-deployment monitoring plans, real-time monitoring, user-input capture, appeal/override, incident response, recovery, decommissioning, and change management — the strongest official bridge from classic ML risk management to agentic/GenAI deployments. The fair limitation (argued by the Cloud Security Alliance's agentic NIST profile, among others) is that NIST guidance remains more general than agent-specific operational playbooks for tool interfaces, hand-offs, and delegated authority — the gap MIDAS fills.
- **MITRE ATLAS** catalogs adversarial techniques (including agent-focused ones). It answers "which techniques to test for," not "is my agent drifting" — necessary for credibility, not an operational detection spec.
- **ISO/IEC 42001** (AI management system) provides the operational/certification structure complementing NIST's risk methodology.
- **NIST's six post-deployment monitoring categories** — functionality, operational, human factors, security, compliance, large-scale impacts — are a useful dashboard-grouping framing (see §5).

## 3. Detection — a layered ensemble

The core lesson: detect drift with an ensemble of monitors, not one dial. Merged method families:

| Method | Signals required | Pros | Cons | Typical use |
|---|---|---|---|---|
| Distribution tests (JS, PSI, KS, Wasserstein, chi-square, KL) | Inputs/outputs over reference + current window | Cheap, early warning, automatable | Detects change, not harm | Input/output drift, tool-call mix shift |
| Model-performance monitoring | Predictions + labels or delayed outcomes | Closest to business impact | Labels delayed or unavailable | Accuracy, precision/recall, quality degradation |
| Online sequential detectors (CUSUM, Page-Hinkley, ADWIN, DDM/EDDM, HDDM) | Streaming metrics or error signals | Fast, suited to runtime alerting | Sensitive to tuning/noise; compute/energy trade-offs | Near-real-time alarms on critical surfaces |
| Behavioural / trajectory | Plans, tool calls, retries, loops, handoffs, completion | Best fit for agents | Needs tracing and often rubrics/judges | Tool misuse, route drift, over-acting/over-thinking |
| Internal-state / activation monitors | Hidden-state deltas, embeddings, memory state | Can catch drift before visible failure | Requires model internals — **likely out of reach for MIDAS's governance/inspection layer** | Prompt-injection/task-drift, deep semantic change |
| Embedding / semantic | Centroid distance, cluster shift, MMD | Earlier than token/schema stats | Harder to interpret; projection artefacts | RAG, memory, topic/intent drift |
| Attribution / counterfactual | Features, traces, baseline window, outcomes | Strong for diagnosis/remediation | Too slow for first-line paging | Root cause, post-incident review |
| Shadow / canary / mirrored traffic | Parallel deployments, live or sampled traffic | Strong pre-release evidence, safe comparison | Extra complexity/cost | Model upgrades, prompt/policy/tool changes |

**Caveat on internal-state monitors (MIDAS-specific):** activation/hidden-state monitoring is a legitimate technique (notably for activation-based task-drift detection), but it requires access to model internals. MIDAS operates at the governance/inspection layer over a graph and does not host model weights, so this family is likely *not* MIDAS-implementable; treat it as context for what a fully-integrated stack could do, not as a MIDAS roadmap item.

**Establishing the baseline — including goal-conditioning (restored).** Use in combination:
- *Goal-conditioned baselines* — expected tool-usage/event/sequence distributions **per declared objective**, so legitimate adaptation isn't flagged while covert change within a stable goal is. This is the field's leading answer to its hardest problem (benign adaptation vs harmful drift look identical), and is retained here deliberately.
- *Initial-window baseline* — capture early decision/tool-usage/coordination patterns; compare subsequent behaviour against them.
- *Reference-trajectory baseline* — build reference trajectories of correct tool choice/sequence/state propagation for critical workflows; score live traces against them.

**Behavioural/trajectory is the primary agent layer.** Output-only monitoring is misleading: agents evaluated only on final-output quality pass materially more cases than full-trajectory evaluation reveals, because the failure surface (tool arguments, state propagation, goal alignment, retries, loops, wasted computation) is at the step level. Core behavioural metrics: task completion, abstention appropriateness, tool-call success, tool-input validity, retry/loop rate, step count, latency, token/cost budget, guardrail trip rate, human-correction rate; where a reference path exists, trajectory edit distance or ordered-step precision/recall. For semantic layers (RAG/memory), embedding drift or **MMD** on vectorised contexts (deploy MMD on sampled/compressed representations — naïve versions are expensive).

**Authority-path detection (MIDAS-specific, first-class — restored).** Replace static RBAC checks with continuous authorization monitoring: maintain delegation-provenance chains distinguishing *authorised transfers* from *unauthorised escalation* (implicit privilege drift), and compile temporal authority policies into finite-state-machine conformance checks over the telemetry stream (e.g., "transfer must be followed by a manager-role approval within 60 seconds"), catching multi-step sequence violations that per-action checks miss. Keep this elevated, not folded into "secondary explanation layers."

**Label latency and label-free estimation (new in v4).** In production, ground truth often arrives late or never, making direct performance monitoring impossible for long stretches. Combine: labelled outcome monitoring where feasible; **unsupervised drift checks** (distribution/sequential); **confidence-based / label-free performance estimation**; and **sampled human review**. Do not design the system assuming labels are promptly available.

**Reward/function, policy, specification, and pipeline drift are first-class operational checks.** The GPT-4o case shows feedback weighting can alter behaviour with no schema change; built-in data-quality metrics (null rate, type errors, out-of-bounds rate) illustrate pipeline drift.

**Composite scoring.** The composite score is a per-surface triage and prioritisation signal, not the primary paging condition. It must always decompose into per-dimension contribution. Paging is triggered by decomposed signal breaches, authority events, safety events, or configured risk-tier rules — not by the composite alone (see Open decisions §4).

**Statistical tooling, platform practice, and the significance caveat.** Standard tests: PSI, KS, JS divergence, chi-square, Wasserstein, KL. Hyperscaler references: **Google Vertex** uses L-infinity (categorical) and JS (numerical), default threshold 0.3 (a platform default, not a standard — use as a seed and tune); **Azure ML** supports JS, PSI, Wasserstein, KS, chi-square plus data-quality and feature-attribution drift; **AWS SageMaker Model Monitor** supports data-quality, model-quality, bias, and feature-attribution drift on scheduled jobs. **Caveat:** distributional tests confuse benign seasonality/traffic-mix with harmful drift, and large samples make trivial changes look statistically significant — weigh **effect size, sample size, and business relevance together**, not a single cutoff.

**Release-time defences (new in v4).** Azure documents blue-green rollout with mirrored traffic and gradual live-traffic shifts; AWS documents shadow variants to validate a candidate stack before promotion. For MIDAS, all model, prompt, and tool changes on high-consequence surfaces should pass canary or shadow checks before full rollout — this exposes drift before it reaches users and keeps reference traces current.

**Cadence, sampling, observability, and privacy.** Vary cadence by risk: streaming/near-real-time sequential checks on action and guardrail metrics for high-consequence surfaces, plus hourly/daily statistical windows on inputs/outputs; daily/weekly windows for lower-risk surfaces; weekly capability roll-ups; CI regression on every prompt/model/tool change. Observability should be **universal in structure but minimised in content**: structured event tracing on all surfaces, combined with redaction, data-minimisation, and selective raw-content retention scaled by risk tier, privacy obligation, and incident need (NIST treats privacy as a trustworthiness characteristic to be balanced; some tracing modes are unavailable under zero-data-retention). Evaluation sampling: 100% behavioural evaluation on critical/irreversible surfaces; 5–10% on medium-risk; 1–2% on low-risk, with automatic upsampling on anomaly. False-positive controls: stable/non-overlapping reference windows, seasonality-aware baselines, cohort slicing, multiple signals rather than one metric, the "two consecutive windows" rule (low-severity only), version-locked human-calibrated LLM judges, frozen benchmark sets, and threshold tuning with model owners to limit alert fatigue.

**Vendor / platform landscape (union of all reports).**
- *Hyperscaler-native*: Google Vertex/Gemini monitoring and trajectory eval; Azure ML drift suite and Microsoft Foundry agent evaluators with sampled continuous evaluation; AWS SageMaker Model Monitor; OpenAI Agents SDK structured traces (generations, tool calls, handoffs, guardrails, metadata at trace/span level), tripwires, sessions, MCP integration, human-in-the-loop; Anthropic task/trial/grader/transcript evaluation with human-calibrated judges and capability suites that graduate into regression suites.
- *Specialist observability*: Arize (AX + Phoenix), Fiddler (JS-distance + PSI), DataRobot (PSI with drift-vs-importance scatter and heatmaps), LangSmith (full-trajectory eval, online quality-drift trends), Langfuse (open-source tracing), Datadog (auto-instrumented spans, anomaly alerting), Evidently / WhyLabs (open-source PSI/KS/Wasserstein), Galileo (dedicated output-drift framing).

## 4. Alerting and escalation

Alerting must be **risk-calibrated, not just metric-calibrated**: detection effort and intervention severity scale with the **stakes** of the task, the **reversibility** of failures, and the agent's **affordances** (the Partnership on AI framing). Anchor severity to action-level risk; combine watch/breach thresholds with graduated containment that preserves operational value.

| Severity | Trigger | Response |
|---|---|---|
| Watch / Advisory | Single-signal *statistical* deviation or mild performance slippage; >~1σ from baseline | Increase sampling, inspect cohorts, notify owners, watch next windows (confirmation rule applies here) |
| Restrictive | Repeat breach or behavioural regression; configured degradation threshold crossed | Narrow tool access, add extra checks, require approval for risky actions; open incident, run replay/regression |
| High | Drift on a regulated/irreversible surface, or an authority-profile breach | Route execution to human review; disable the risky path; notify engineering + security + governance |
| Critical / Circuit-breaker | Unsafe execution, confirmed harmful trajectory, or a single privilege/authority violation | Escalate immediately (no confirmation window); rollback, tool disablement, or circuit-break; revoke credentials; preserve traces; formal incident response |

Distinguish **circuit breakers** (automated, threshold-triggered) from **kill switches** (manual). The confirmation-window discipline applies to low-severity statistical anomalies, not to high-severity safety/security/authority events, which page on first occurrence. HITL is mandatory for irreversible, regulated, or authority-expanding actions, and automated mitigation should be pre-wired, not improvised: block/short-circuit risky inputs and tool calls; retry with a safer plan/model; degrade autonomy by adding approval; disable a tool path; reset memory; roll back prompt/model/policy versions; or gracefully shut down. Pre-wire concrete triggers: financial-velocity caps, repeated-identical-tool-call loop detection, iteration/budget ceilings, escalation TTL + cooldown. **Default-deny and escalate immediately on any privilege-elevating goal/authority transition.**

Deterministic triage sequence: confirm the version change → inspect cohorts → compare against the reference baseline → replay the worst traces → localise to data/tool/policy/model/prompt → choose remediation.

## 5. Visualisation and dashboards

| Visualisation | Best for | Caveat | MIDAS use |
|---|---|---|---|
| Time-series with thresholds + uncertainty bands | Onset, persistence, severity | Can hide cohort effects | Per-surface drift score over time; default spine |
| Heatmap | Portfolio view across many units | Weak on causality | Capability overview of drifting surfaces |
| Cohort analysis | Compare by model, tenant, geography, policy, version | Easy to over-slice | Detect one prompt version drifting while others hold |
| Distribution overlay | Baseline vs current input/output/tool distributions | Needs stable reference | Input drift, tool-choice drift |
| Reliability / calibration plot | Whether confidence aligns with correctness | Only where labels exist | Surfaces with probabilistic risk scoring |
| Embedding drift map | Topic/semantic shifts | Diagnostic only; projection artefacts; pair with exemplars | RAG and memory drift |
| Sankey / flow | Route, handoff, tool, and authority drift | Clutter at scale | **Signature MIDAS view** (route *and* authority/delegation flow) |
| FSM conformance trace | Temporal/authority policy violations | Needs compiled policy | Authority and policy drift |
| Composite score + contribution breakdown | Per-surface triage and prioritisation | Not the primary paging condition | Governance/triage headline |
| Trace replay panel | Root cause and audit | Expensive to store | Inspect the exact run that triggered the alert |

**Sankey serves two complementary purposes MIDAS should offer together:** (a) *route drift* — intent → planner → tool → handoff → outcome, valuable precisely when output text looks fine but the route quietly worsens (less standard in classical ML monitoring, highly appropriate here); and (b) *authority/delegation flow* — delegation chains and permission inheritance across spawned sub-agents, with deviating ribbons flagging authority-path divergence.

**Three-tier MIDAS Explorer layout:** Tier 1 — capability heatmap and trend (counts and severity of drifting surfaces). Tier 2 — ranked decision-surface watchlist with drift score, severity, and recent change. Tier 3 — selected-surface drill-down: observed-vs-expected time-series (shaded deviation = the area between observed and expected, not observed-to-zero), distribution overlays, trajectory/authority Sankey, tool/action table, contribution attribution, confidence bands, and trace replay.

**Audience layering:** engineers need surface traces and regressions; product owners need cohort/journey views; governance teams need exposure, reversibility, and control evidence (NIST's six categories as the governance grouping).

**Honest vs misleading conventions (product requirements, not options):** show the baseline/reference explicitly; annotate thresholds and rationale; show data volume and grey out unreliable low-volume periods; overlay deployment/external events for causal correlation; surface judge and projection uncertainty; keep drift, degradation, and attack visually distinct. Avoid cherry-picked windows, hidden baselines, unjustified thresholds, 2D-embedding proximity presented as ground truth, and aggregating away the long-conversation tail where drift hides.

## 6. Recommendations for MIDAS

1. **Model the four nested units** as MIDAS design decisions, not field standards: decision surface = chosen primary alert key; authority profile = primary explanatory lens for high-severity alerts; agent = route/execution-quality monitoring; capability = roll-up. Use an agency-risk index per agent to drive risk-proportional monitoring intensity — initially a qualitative tier, later formalised into a scored index once enough telemetry exists — and goal-conditioned baselines so benign adaptation isn't flagged.
2. **Instrument first, minimise content.** Tag every run with a `decision_surface_id`, tool spans, guardrail events, delegation/authority events, version metadata, and environment observations, retaining enough provenance to replay the triggering run — but apply redaction, data-minimisation, and risk-tiered raw-content retention to meet privacy obligations. Align to OpenTelemetry / OpenInference conventions.
3. **Define baselines and risk tiers per decision surface**, not per business capability. Identify the first 10–20 critical/irreversible surfaces; attach rubrics, reference trajectories, authority policies (as FSMs), and risk/reversibility labels.
4. **Deploy the ensemble detector stack:** cheap distribution tests + sequential runtime detectors (confirmation rule for low-severity excursions) + sampled trajectory evaluation + embedding/MMD semantic checks + goal-vs-stated-goal interrogation + authority-path/delegation-provenance checks. Plan for label latency: pair labelled monitoring with unsupervised drift checks, confidence-based estimation, and sampled human review. Highest-leverage governance signals: tool-call distribution shift, authority-path divergence, goal mismatch.
5. **Add governance controls:** tiered alerting (Watch / Restrictive / High / Critical) with default-deny on privilege elevation; confirmation windows for low-severity statistical anomalies but immediate escalation for high-severity safety/security/authority breaches; HITL for sensitive actions; canary/shadow releases for model/prompt/tool changes; rollback/circuit-breakers for critical surfaces.
6. **Build the three-tier dashboard** with the §5 idioms and honesty conventions as hard requirements; lead the governance view with the composite-plus-contribution timeline and the authority/delegation Sankey.
7. **Map to standards.** Publish MIDAS's drift model against OWASP ASI10/ASI03/ASI01, NIST AI RMF + GenAI Profile, MITRE ATLAS, and ISO/IEC 42001, and publish MIDAS's own precise, versioned definitions rather than assuming field consensus.

**MIDAS entity → metrics → primary-visual mapping:**

| MIDAS entity | Metrics to add | Primary visual |
|---|---|---|
| Decision surface | Task success, abstention appropriateness, tool success, loop rate, guardrail trips, observed-vs-expected drift score | Time-series + trace replay |
| Authority profile | Approval bypasses, privilege escalations, irreversible actions without approval, delegation depth | Sankey + authority heatmap |
| Agent | Latency, retries, token/cost budget, tool mix, memory growth, prompt/version drift | Cohort plots + route chart |
| Capability | Drifting-surface count, severity mix, exposure, top contributors | Heatmap + portfolio time-series |

**MIDAS KPIs:** per-surface drift score (composite of data/trajectory/outcome deviation, always decomposable by component); contribution attribution (top feature, tool, prompt version, or authority path); time-to-detect and time-to-contain; alert precision and false-positive rate (alert recall where labels exist); route-degradation indicators (retry rate, loop rate, off-canonical-path rate, failed-tool-call rate); human-override indicators (approval/rejection/rollback rate); root-cause indicators (top shifted inputs, changed attributions, changed reward/feedback policy, changed dataset provenance).

**Implementation pitfalls:** label latency (ground truth late or absent — see §3); sampling bias and non-stationary environments; multi-agent coordination failures hidden by aggregate output quality (monitor coordination, handoffs, memory, planning separately); uneven context use ("lost in the middle"); and privacy/data-minimisation trade-offs (trace universally in structure, retain raw content selectively).

---

## Open decisions and calibration notes

The sources are substantially aligned with no factual contradictions. Some items below are resolved and recorded as MIDAS decisions; others are open decisions or calibration notes to revisit as telemetry accrues.

1. **Threshold lineage conflict (decide per statistic).** Google's platform default (investigate at **0.2** over two consecutive windows; **0.3** severe only with behavioural degradation) versus the PSI credit-risk convention (**<0.1** none, **0.1–0.25** minor/moderate, **>0.25** major). Different statistics from different lineages, not interchangeable. Pick one convention **per statistic**, treat platform defaults as seed values, and tune by false-positive rate, criticality, and reversibility.

2. **Primary alerting unit — resolved as a MIDAS design decision.** Decision surface = unit of detection (primary alert key); authority profile = primary explanatory lens for high-severity alerts; agent = execution-quality monitoring; capability = roll-up. This is a MIDAS product-design choice, **not an externally standardised unit** — agent-evaluation research remains multi-perspectival. Record it as a deliberate choice; it determines deduplication, roll-up, and on-call routing.

3. **Definitional scope of "drift" (reflected in the definition; one decision remains).** The definition separates observed runtime divergence from attributed cause. Remaining product decision: does MIDAS *claim to detect* underlying model/concept drift, or only *attribute* to it? Recommendation: claim detection of behavioural/authority/trajectory drift; attribute to model/concept/data/reward drift as a likely cause where evidence supports it; do not claim governed detection of underlying model drift unless MIDAS instruments model versioning.

4. **Composite score stance — resolved.** The composite score is a per-surface triage and prioritisation signal, not the primary paging condition; paging is triggered by decomposed signal breaches, authority events, safety events, or configured risk-tier rules. The composite must always decompose into per-dimension contribution. This preserves the UI value of a single score without letting it become an unexplained governance artefact. Carried-forward product constraint: the compact Drift panel currently renders the composite and contribution percentages **as frontend demo/provisional, not backend-governed or reconstructible**; do not promote it to "governed/reconstructible" until the per-dimension contributions are themselves production-backed.

5. **Sampling/cadence numbers are unvalidated starting points.** The rates (100/5–10/1–2% behavioural evaluation by risk tier; 0.2 two-window; ~3pp warn / 5pp page; 14-day baselines) are reasonable but not field-validated for MIDAS's workload. Adopt as provisional; tune from observed false-positive rates. Trigger to revisit: if false positives on benign adaptation exceed ~5–10%, move from threshold-only to goal-conditioned baselining.

6. **Provenance of headline statistics and citations (verify before external use).** Several quantitative claims are simulation-based or vendor-sourced. Specific items to confirm: the **"Rath, Agent Drift"** author attribution; the **"e-valuator"** online-verifier as a real named work; and the **HDDM_W/ADWIN vs DDM/EDDM/Page-Hinkley** comparative result (workload-specific; do not generalise). Verify against primary sources before MIDAS-facing materials, CNCF submissions, or Athena collateral.

7. **Authority-path *detection method* is the least mature element.** Elevating authority/permission-path divergence to first-class is right, but it has the least off-the-shelf tooling and weakest standardisation. Delegation-provenance diffing, privilege-elevation detection, and FSM authority-conformance will largely be MIDAS's own contribution — plan for original engineering, and a likely source of defensible differentiation.

8. **Internal-state/activation monitoring is likely out of MIDAS's reach.** It can catch drift before visible failure but requires model internals; MIDAS sits at the governance/inspection layer and does not host weights. Decide explicitly whether MIDAS ever ingests activation/hidden-state signals from instrumented agents, or treats this family as out of scope (the likely answer).

9. **Terminology is unstandardised.** "Agentic drift," "agent drift," "behavioural drift," "goal drift," "task drift," "mission drift," and "cognitive degradation" are used loosely and interchangeably. Publish MIDAS's own precise, versioned definitions and map external terms onto them.
