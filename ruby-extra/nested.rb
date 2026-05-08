# nested constructs: classes, modules, singleton classes, method defs

# class in class
class Outer
  class Inner
    def method
      42
    end
  end
end

# module in class
class Container
  module Mixin
    def mixed
      "mixed"
    end
  end
end

# class in module
module Namespace
  class Wrapped
    def value
      1
    end
  end
end

# singleton class on self
class << self
  def helper
  end
end

# singleton class on arbitrary expression
obj = Object.new
class << obj
  def custom
  end
end

# singleton method definitions
def Array.wrap(x)
  [x]
end

def obj.instance_method
end

# setter method definitions
class Setters
  def foo=(val)
    @foo = val
  end

  def []=(key, val)
    @store ||= {}
    @store[key] = val
  end
end

# operator method definitions
class Operators
  def +(other)
  end

  def <=>(other)
  end

  def ==(other)
  end

  def [](idx)
  end
end

# private/protected/public (parsed as plain method calls)
class Visibility
  private
  def secret
  end

  protected
  def semi_secret
  end

  public
  def visible
  end
end

# module_function
module Functions
  def func1
  end
  module_function :func1
end

# unary operator methods (+@, -@)
class UnaryOps
  def +@
    self
  end

  def -@
    -self
  end
end

# singleton method on global
def $stdout.custom_log(msg)
  msg
end

# top-level scope method call
::TopLevel.method
::Foo

# endless method (def foo = expr)
def add(x) = x + 1
def greet(name) = "Hello, #{name}"
def no_args = 42
def with_parens() = nil
