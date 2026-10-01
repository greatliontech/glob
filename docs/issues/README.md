# Issue docs — deferred follow-ups

Tracked deferrals carrying a `Lands:` trigger. On resolution, the
load-bearing rationale is promoted inline to the spec / a test, and the
doc is deleted (git holds history).

| slug | summary | Lands |
|------|---------|-------|
| [compile-options-for-git-matching](compile-options-for-git-matching.md) | git-go must match paths exactly as git's wildmatch does and will use this library's engine for it; each difference from git becomes a compile-time option (bytes as the unit, class negation, named classes, braces, case folding, star rules), with git's behaviour as one preset | git-go's first plan slots a chunk that matches paths against git patterns (pathspecs, ignore rules or attribute rules) |
