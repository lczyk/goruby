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

## dispatch cleanup -- done

`callMethodLegacy` is gone. Class-receiver dispatch (new, superclass,
name, define) lives on ModuleClass. Comparable derivations live on
ObjectClass, gated on `<=>`. Enumerable derivations live on
ObjectClass, gated on `include Enumerable` + `each`. `callMethod`
shrunk to: user class-method check, Send, NoMethodError.

Block-aware dispatch also lives on class chains now.
`callMethodWithBlockImpl` and `callEnumerableBlock` are gone.
`callMethodWithBlock` builds a `goBlockMarker` (carries the invoke
callback + the AST BlockExpression) and routes through `object.Send`
just like the no-block path. Per-class block methods are registered
in `z_block_*.go` files (named with a `z_` prefix to ensure their
init runs after the non-block per-class files, since several names
overlap and the last registration wins). Enumerable block-form
derivations live on ObjectClass alongside the no-block ones, with
the Fn inspecting the block payload to pick a path.

## perf -- harvested

- **small-integer cache** -- `NewInteger` returns shared pointers
  for values in [-128, 1152]. Already in place; no further work.
- **inline env bindings** -- `Environment` holds up to `envInlineCap`
  bindings inline (linear scan); only promotes to a map on overflow.
  Cut method-call alloc roughly in half on tight workloads
  (EvalRecursiveFib: 9047 -> 5110 allocs, -43%).
- **Range#each / #map direct iteration** -- skip `rangeToSlice` for
  Integer-bound ranges. Modest win (RangeIteration: 538 -> 532).
- **<=> cache on Class** -- `LookupSpaceship` memoises
  `LookupMethod("<=>")`, version-gated. Marginal CPU win
  (~5ns/comparison), alloc-neutral. Infrastructure exists for
  caching other hot methods (==, hash, each) the same way.

## perf -- remaining ideas

ranked by expected impact / risk ratio. (Inline call-site cache is
the biggest single win remaining -- listed under [class machinery]
above since it builds on the eigenclass machinery's version-gated
lookup story; mentioned here for completeness.)

- **per-iter args slice alloc on block yield** -- every
  `iterStep(invoke, []RubyObject{v})` heap-allocates the 1-slot
  slice because the closure boundary defeats escape analysis. Two
  paths:
  1. Change `blockCallback` signature to `func(args0 RubyObject,
     rest ...RubyObject)` -- scalar fast path for the 1-arg case,
     covers ~all yields. Touches every block callback and
     `invokeBlock`. Medium scope.
  2. Add a reusable buffer field on the calling loop frame --
     `buf := make([]RubyObject, 1)` once outside the loop, reuse
     inside. ~1 alloc per loop instead of N. Smaller but pollutes
     every counter-loop body with a buf decl.
  Either way: kills 100+ allocs per Range/times/each loop.
- **method-frame env pooling** -- callEnv per method call accounts
  for ~half of EvalRecursiveFib's 5110 allocs. Pool envs in a
  sync.Pool, reset on release. Risk: envs alias via DefEnv closures
  (the def-time env survives, but the call env is short-lived);
  need a strict audit of who retains callEnv after the call
  returns. Big win if the audit lands clean.
- **string concat builder** -- `buf + "..."` in interpolation /
  loop-building paths allocates a fresh `String` per `+`. A
  bytes-level builder shared per expression would drop most of
  EvalStringInterpolation's allocs. Lower priority -- this isn't
  a hot path in real corpus.
- **kwargs map alloc** -- when no kwargs are passed (most calls),
  the dispatcher still threads `nil` through but the receiving
  method may build an empty `map[string]RubyObject`. Audit
  bindParams for unnecessary map allocs.
- **dispatcher-side arg slice for 0-arg calls** -- `first`, `to_s`,
  `inspect`, etc. all build `args = []RubyObject{}` at the call
  site. A shared package-level `emptyArgs` would cover these.
  Tiny win, but it's free.
