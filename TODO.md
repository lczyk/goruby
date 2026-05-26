# todo

## class machinery -- remaining

- **inline call-site cache** -- `object.Class.Version` already bumps on
  every (re)def. attach `(classPtr, methodPtr, version)` to ast call
  nodes; check version each call, refill on mismatch. mri-style
  monomorphic cache. biggest single perf win available.
- **multi-level super chain w/ defined_class tracking** -- current
  `evalSuper` walks from the receiver's class each time, so two
  consecutive `super` calls hit the same method and infinite-loop.
  needs a per-frame "defined class" so each super starts above the
  class that defined the currently-executing method.
- **Class object metaclass** -- `Foo.singleton_class` currently returns
  ClassClass as a coarse stand-in. real per-class singleton would
  enable `def Foo.foo` via `class << Foo; def foo; end; end`
  shorthand on raw Class receivers.

## class machinery -- done

- **method visibility** -- `private` / `protected` / `public` (bare +
  arg form), `send` bypass vs `public_send` enforce, kin-check for
  protected. lives in `eval_method` + `block_core` +
  `builtin_object_methods` + `builtin_universal_zblock`.
- **eigenclass / singleton methods** -- per-Instance `SingletonClass`
  lazily materialised; `class << obj`, `def obj.foo`, `class << self`,
  `singleton_class`, `define_method`, `singleton_methods` all routed
  via `Send` walking the singleton class chain before the canonical
  class.

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
