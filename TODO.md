# todo

## class machinery -- deferred

bolt-on items from the dispatch refactor (commits `f7d351e`, `68cd461`).
each is cheap to add on top of the existing chokepoints; nothing
preordained forbids them.

- **method visibility** -- add `Visibility()` to `object.RubyMethod` +
  one check in `object.Send`. mark builtins private where appropriate
  (`puts`, `raise`, ...). distinguishes `send` from `public_send`
  properly.
- **eigenclass / singleton methods** -- introduce per-object singleton
  class. swap `evaluator.dispatchClass` / `classOfRaw` to consult the
  singleton when present, falling through to the canonical class
  otherwise. enables `def obj.foo` and class-level method definitions
  via the singleton.
- **inline call-site cache** -- `object.Class.Version` already bumps on
  every (re)def. attach `(classPtr, methodPtr, version)` to ast call
  nodes; check version each call, refill on mismatch. mri-style
  monomorphic cache. biggest single perf win available.

## dispatch cleanup -- mechanical follow-on

`evaluator/callMethodLegacy` still hosts per-name bodies for Array /
Hash / Range / String. their `<type>_methods.go` files register thin
adapters that delegate back. inlining the bodies + deleting the matching
type-switched arms collapses the legacy fn further toward zero. pure
churn; no design decisions.

dead arms also sit in callMethodLegacy for fully-migrated types
(Integer / Float / Symbol / Nil / Boolean) -- Send always wins so the
arms never fire. safe to delete.
