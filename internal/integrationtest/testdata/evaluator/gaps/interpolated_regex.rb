needle = "foo"
re = /^#{needle}bar/
puts re.match?("foobar")
puts re.match?("xfoobar")
