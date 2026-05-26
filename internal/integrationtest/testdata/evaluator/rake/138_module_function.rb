# module_function makes subsequent defs available both as private
# instance methods and as class-level methods on the module. Rake's
# Cleaner module relies on the dual exposure.

module M
  module_function

  def shout; "HI"; end
  def whisper; "shh"; end
end

# Class-method form: Module.method works.
p M.shout                                   #=> "HI"
p M.whisper                                 #=> "shh"

# Including the module exposes the instance copies (which are
# private). The instance method is callable from inside the
# including class but not as a public method on instances.
class C
  include M
  def boom; shout; end
end
p C.new.boom                                #=> "HI"

# (MRI marks the included copy private so explicit-receiver call
# raises; goruby's module_function shim doesn't yet enforce that.
# The dual class-method exposure is the main feature this driver
# pins.)
