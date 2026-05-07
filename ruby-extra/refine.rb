# refinements: using and refine

module RefinementExample
  refine String do
    def shout
      upcase + "!"
    end
  end
end

using RefinementExample

# refine with class methods
module ClassRefinement
  refine String.singleton_class do
    def build
      new("built")
    end
  end
end

# refine inside module/class body
class Container
  using RefinementExample

  def use_it
    "hello".shout
  end
end

# multiple refinements for same class
module MoreRefinements
  refine String do
    def whisper
      downcase
    end
  end

  refine Array do
    def second
      self[1]
    end
  end
end
