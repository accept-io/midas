# Search

The workbench toolbar carries a search input (centre of the toolbar). It
filters nodes in the **active** lens by:

- Node label (case-insensitive substring).
- Node ID (case-insensitive substring).

## What search does NOT do

- It does not cross lenses — switching from the Context Graph to the
  Authority Graph re-scopes the search.
- It does not query evidence envelopes; for that, use the workbench
  Evidence tab.
- It does not change selection automatically; matches are highlighted but
  you still click to select.

## Tips

- Clear the input with `Escape` or by deleting the value.
- An empty search shows all nodes.
- For very large graphs, use the Layers control to hide kinds you don't
  need (e.g. fail-mode policies) before searching.
