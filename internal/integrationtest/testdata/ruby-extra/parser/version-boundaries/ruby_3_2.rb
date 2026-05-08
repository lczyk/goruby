# syntax introduced in ruby 3.2 (fails 3.1, passes 3.2+)
#
# boundary: 3.1 -> 3.2
#
# changes exercised:
#   - anonymous rest forwarding: def f(*); g(*); end
#   - anonymous keyword rest forwarding: def f(**); g(**); end
#   - anonymous rest + kwrest combined
#   - anonymous rest in array literal / kwrest in hash literal

# --- anonymous rest forwarding ---
def forward_rest(*)
  other(*)
end

def other(*args, **kwargs)
  [args, kwargs]
end

# anonymous rest with named params
def mixed_rest(first, *)
  first
end

# --- anonymous keyword rest forwarding ---
def forward_kwrest(**)
  other(**)
end

# anonymous kwrest with named params
def mixed_kwrest(a:, **)
  a
end

# --- combined anonymous rest + kwrest ---
def forward_both(*, **)
  other(*, **)
end

# --- anonymous rest in array literal ---
def to_array(*)
  [1, *, 2]
end

# --- anonymous kwrest in hash literal ---
def to_hash(**)
  {base: true, **}
end

# --- all anonymous forwarders together ---
def everything(*, **, &)
  other(*, **, &)
end
