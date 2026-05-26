# minversion: 2.6
# rung 7 -- Kernel#system.
#
# rake's `sh` helper inside a task delegates to Kernel#system.
# Run a tiny shell-out and check that its stdout reaches the parent
# stdout, plus the return-value semantics (true on success).
#
# Sticks to `echo` + a no-op exit pattern that works under the
# sandboxed shell the test harness runs under.

system("echo hello from system")   #=> hello from system
puts system("exit 0")              #=> true
puts system("exit 1")              #=> false
