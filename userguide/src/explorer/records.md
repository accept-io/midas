# Records

A **record** in MIDAS is the canonical typed object behind a node in either
graph. Every Authority Graph node (business service, decision surface,
authority profile, etc.) has a backing record; selecting the node and
opening the Inspector tab shows its direct attributes.

## View Record

The Inspector tab carries a **View Record** affordance for kinds that have
a navigable record view. Clicking it routes to a dedicated record screen
for the selected entity.

Not every node kind has a record view; for those, the Inspector + the
Technical details disclosure is the full surface.

## Finding a record by ID

If you have an envelope ID or a record ID and want to land on the
matching graph view, paste the ID into the workbench search input. The
search matches against IDs and labels across the active lens.

See also: [Evidence Envelopes](../evidence/evidence-envelopes.md) for the
record that justifies a single decision.
