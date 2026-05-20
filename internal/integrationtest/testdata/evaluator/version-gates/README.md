# evaluator version-gate corpus

each file targets one ruby major.minor boundary, mirroring the parser's
`testdata/ruby-extra/parser/version-boundaries/` layout. one file per entry in
`token.knownBoundaries`. each file declares its minimum version via a header
comment of the form `# minversion: 2.7` (recognised by
`scripts/eval-corpus-oracle`).

intent: when the evaluator gains version-aware behaviour, each file becomes a
positive test for a feature introduced (or whose semantics changed) at that
boundary. for parser-only gates the file may simply parse-and-print -- the
oracle will run it under MRI and confirm.

current files are stubs. flesh out as evaluator features land. the manifest is
in `evaluator-version-boundaries.tsv` two dirs up.
