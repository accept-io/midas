# ADR-0011: Standards Alignment

## Status

Accepted

## Date

2026-06-10

## Context

MIDAS is a governance tool intended for CNCF submission. For such a product, an
explicit mapping onto recognised agentic-security and AI-risk standards is much
of what confers external credibility. The v5 briefing notes this standards layer
keeps getting dropped from drafts and restores it as a first-class design
position.

## Decision

MIDAS **publishes its drift model explicitly mapped onto the agentic-security
and AI-risk standards**:

- **OWASP Top 10 for Agentic Applications** — primary matches **ASI10 Rogue
  Agents**, **ASI03 Identity & Privilege Abuse**, **ASI01 Agent Goal Hijack**
  (and ASI09 Human-Agent Trust Exploitation); the **Least Agency** principle
  organises the Authority lens; **MAESTRO** as the companion threat-modelling
  method.
- **NIST AI RMF 1.0** (GOVERN / MAP / MEASURE / MANAGE) plus the **NIST GenAI
  Profile** for post-deployment monitoring, real-time monitoring, appeal/
  override, incident response, recovery, decommissioning, and change
  management — the strongest official bridge, whose agent-specific operational
  gap MIDAS fills.
- **MITRE ATLAS** — answers "which techniques to test for," for credibility, not
  as an operational detection spec.
- **ISO/IEC 42001** — operational/certification structure complementing NIST.
- **NIST's six post-deployment monitoring categories** (functionality,
  operational, human factors, security, compliance, large-scale impacts) as the
  governance dashboard grouping.

## Consequences

- The standards mapping is maintained as published MIDAS collateral; the
  authority lens ([ADR-0007](0007-authority-path-drift-first-class.md)) maps to
  ASI03/ASI10 and Least Agency.
- The governance dashboard groups along NIST's six categories
  ([ADR-0012](0012-visualisation-principles.md)).
- MIDAS positions itself as filling NIST's agent-specific operational gap (tool
  interfaces, hand-offs, delegated authority).

## Alternatives Considered

- **Skip the standards mapping** (as some drafts did). Rejected: it is
  disproportionately important for a CNCF-submitted governance product.
- **Pick a single standard.** Rejected: each covers a different facet
  (techniques vs lifecycle vs certification vs agentic threats); the value is
  the combined mapping.

## Source

v5 briefing §2 (*Standards and governance mapping*); Executive summary (third
design position); §6 item 7. See
[Agentic AI Behavioural Drift — Consolidated Briefing for MIDAS (v5)](agentic-ai-behavioural-drift-consolidated-briefing-v5.md).
