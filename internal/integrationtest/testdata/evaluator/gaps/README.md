# gaps/

one fixture per known evaluator gap. each `.rb` file:

- runs unmodified under MRI 2.6+ and produces a `.expected` output (once generated)
- is listed in `internal/integrationtest/evaluator.skip` under phase `eval`, so `TestEvaluatorCorpus` skips it before consulting `.expected`
- demonstrates the gap minimally -- no embellishment, just enough to fail today

flip workflow once a gap is closed: drop the matching line from `evaluator.skip`, regenerate `.expected` via `scripts/eval-corpus-oracle`, ensure goruby matches. these fixtures double as the regression coverage for the fix.

gaps inventory was gathered by greping evaluator/ + object/ for `not yet supported|stub|no-op|workaround|not implemented|punt` plus the pinned items in `evaluator/gap_probes_test.go`.
