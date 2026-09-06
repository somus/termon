# Temporary renderer backport — TERM-71

This directory is copied from `github.com/charmbracelet/ultraviolet`
`v0.0.0-20260903151058-ae99b731b8c5`, commit
`ae99b731b8c580350966069bc83037227ede021c`. The upstream MIT license is retained.
Upstream HEAD still resolved to this commit when checked for this investigation.

The only production-source change is in `TerminalRenderer.putRange`:

```diff
- if same > end-start {
+ if same > inline {
```

The old condition cannot hold when an unequal cell ends an unchanged run inside
the inclusive `[start,end]` range. Consequently the renderer re-emits unchanged
interior cells, including their color switches, instead of moving the cursor past
them. `inline` is the cursor-movement cost already computed by this function.
The correction reuses the existing emission and cursor-positioning code; it does
not drop frames, reorder ANSI writes, change artwork, or alter terminal capabilities.

Termon's root `go.mod` retains the exact upstream requirement and uses a local
replacement so ordinary Go commands and the container build use the same fix.
The repository check script includes this module's tests under the race detector.

Remove this replacement and directory when an upstream version containing the
fix is selected and the rendering equivalence / SSH measurements still pass.
Do not accumulate unrelated dependency changes here. Upstream contribution has
not been published from this checkout.
