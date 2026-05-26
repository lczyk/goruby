# Singleton def on an @ivar receiver: `def @x.foo` now looks up @x
# on the enclosing self's instance variables when @x isn't otherwise
# bound. Rake's test suite uses this shape extensively to stub
# methods on per-test fixtures (e.g. `def @app.unix?; true; end`).

class Holder
  def initialize
    @app = Object.new
    def @app.unix?; true; end
    def @app.width; 1234; end
  end
  attr_reader :app
end

h = Holder.new
p h.app.unix?                               #=> true
p h.app.width                               #=> 1234

# Singleton def via local-variable receiver also works (regression
# check for the existing path).
obj = Object.new
def obj.greet; "hi"; end
puts obj.greet                               #=> hi

# (No downstream rake suite executed here -- with capture_output now
# aliased, some option tests yield indefinitely. Pin only the
# singleton-def behaviour.)
