# gaps/

one fixture per known-or-once-known evaluator gap. each `.rb` file:

- runs unmodified under MRI 2.6+
- has a sibling `.expected` (generated from `#=>` markers via
  `scripts/eval-corpus-expected`, or directly from MRI output via
  `scripts/eval-corpus-oracle`)
- demonstrates the gap minimally -- no embellishment

**default state is live coverage.** historically each fixture landed
skip-listed and flipped to live once the gap closed. now most live
fixtures originally came from gap-closure work, so they sit as plain
regression tests and the skip-list (`evaluator.skip`) is usually
empty. when a NEW gap shows up, the workflow is:

1. add the minimal `.rb` here w/ `#=>` markers
2. generate `.expected`
3. iff goruby fails today, list the fixture in `evaluator.skip` w/
   a one-line reason
4. fix the gap
5. drop the skip-list entry; the fixture becomes regression coverage

the inventory grew organically: greping evaluator/ + object/ for
`not yet supported|stub|no-op|workaround|not implemented|punt`,
the pinned items in `evaluator/gap_probes_test.go`, and ad-hoc MRI
divergence probes (recent: String/Array/Hash/Numeric/Enumerable
surface sweep).
