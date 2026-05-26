# minversion: 2.6
# Pin super inside a define_method body. Minitest::Mock uses this
# pattern: install proxy methods via define_method, fall back to
# super when there's no expectation registered for the call.

class Base
  def greet(name)
    "base hello #{name}"
  end

  def shout(msg)
    msg.upcase
  end
end

class Polite < Base
  [:greet, :shout].each do |op|
    define_method(op) do |*args|
      # Wrap super with a polite suffix. Implicit-args form would
      # also work; use explicit for clarity.
      result = super(*args)
      "#{result}, please"
    end
  end
end

p = Polite.new
puts p.greet("world")                       #=> base hello world, please
puts p.shout("hi")                          #=> HI, please

# Plain Base.new keeps the original behaviour.
b = Base.new
puts b.greet("world")                       #=> base hello world
puts b.shout("hi")                          #=> HI
