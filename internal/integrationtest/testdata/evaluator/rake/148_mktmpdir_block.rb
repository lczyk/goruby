# Dir.mktmpdir with a block yields the path to the block, removes
# the dir on block exit, and returns the block's value. Previously
# the block was ignored and the path was returned without cleanup.

require "tmpdir"

outer_path = nil
result = Dir.mktmpdir do |path|
  outer_path = path
  p File.directory?(path)                   #=> true
  "block-result"
end
p result                                    #=> "block-result"
p File.directory?(outer_path)               #=> false

# No-block form still returns the path (uncleaned, for the caller
# to manage).
p Dir.mktmpdir.is_a?(String)                #=> true
