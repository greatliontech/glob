# Issue docs — deferred follow-ups

Tracked deferrals carrying a `Lands:` trigger. On resolution, the
load-bearing rationale is promoted inline to the spec / a test, and the
doc is deleted (git holds history).

| slug | summary | Lands |
|------|---------|-------|
| [git-profile-and-byte-mode](git-profile-and-byte-mode.md) | git-go must match paths exactly as git's wildmatch does and will use this library's engine for it; that needs matching over bytes and a compile-time profile carrying git's syntax | git-go's first plan slots a chunk that matches paths against git patterns (pathspecs, ignore rules or attribute rules) |
