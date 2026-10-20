# Changelog

## v0.2.0

Disabled flags now return the off value for their type instead of defaultValue (a flag with default=true could not be turned off), an enabled flag at 0% rollout is off, a matched rule with no value turns a boolean flag on, and a disabled JSON flag returns {} (was None/nil). Same semantics as the Node, JS and React SDKs 0.2.0 and the dashboard preview. Adds a shared evaluation contract (tests/contract) and corrects API key examples (keys start with dev_, stg_ or prod_).

## v0.1.0

Initial release.
