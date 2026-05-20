# evaluator integration corpus

small focused ruby programs that exercise individual evaluator features. each
program is self-contained and runnable under MRI as a sanity reference.

## conventions

- one feature per file. keep files short (under ~30 lines).
- mark observable values inline with `#=> value`. mirrors the pickaxe / irb
  convention so the harness can grep for expectations later.
- when the file calls `puts`, the printed text is the value before any newline.
  e.g. `puts 1 + 2  #=> 3` means stdout line is `3`.
- if expected output is multi-line and harder to attach per-line, append a
  trailing `# expected stdout:` comment block.
- no `require`, no stdlib beyond `Kernel#puts`, `Kernel#p`, `Kernel#raise`.
- avoid features the evaluator doesn't aim at yet (regex, ranges, threads, IO).

## layout

- `literals/` -- atomic value forms
- `arithmetic/` -- operators, precedence, comparison, logical
- `variables/` -- locals, globals, instance vars, multiple assignment
- `control_flow/` -- if/unless/while/until/case
- `methods/` -- def, args, return, recursion
- `blocks/` -- yield, iterators, procs, lambdas
- `classes/` -- definition, inheritance, attr accessors
- `modules/` -- definition, include, mixin
- `strings/` -- interpolation, basic methods
- `arrays/` -- access, basic methods
- `hashes/` -- access, basic methods
- `exceptions/` -- raise / rescue / ensure
- `self_kw/` -- `self` semantics in various contexts
