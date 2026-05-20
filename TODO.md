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

## dispatch cleanup -- nearly done

Array / Hash / Range / String bodies now live in per-type helper fns
(`callArrayMethod`, `callHashMethod`, `callStringMethod`, and Range's
inline form in `range_methods.go`). callMethodLegacy is down to a
Class-receiver bridge + the Instance Comparable / Enumerable
derivations + NoMethodError. removing the Instance branch requires
either:

- moving `comparableFromSpaceship` / `callEnumerable` to register on
  the relevant module class (Comparable / Enumerable) at the same
  inheritance level as user instance methods so Send finds them; or
- keeping callMethodLegacy as the small derivation host and routing
  Send misses through it from callMethod (current shape).

callOnClass for `Foo.new` / `Foo.kind` similarly wants to live on
ClassClass's instance-method set; deferred until eigenclass lands so
class-method dispatch can use the same machinery.
