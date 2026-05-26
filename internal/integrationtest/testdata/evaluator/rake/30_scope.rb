# minversion: 2.6
# Exercise Rake::Scope -- a LinkedList subclass that rake uses to
# track namespace nesting. Tests path / path_with_task_name / trim
# and the EmptyScope null object.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake/linked_list"
require "rake/scope"

# Empty scope.
empty = Rake::Scope::EMPTY
puts empty.empty?                          #=> true
puts empty.path                            #=>
puts empty.path_with_task_name("build")    #=> build

# Built via make then conj to mirror namespace nesting: outer first.
s = Rake::Scope.make("outer")
puts s.path                                #=> outer
puts s.path_with_task_name("compile")      #=> outer:compile

# Push an inner namespace.
inner = s.conj("inner")
puts inner.path                            #=> outer:inner
puts inner.path_with_task_name("compile")  #=> outer:inner:compile

# trim drops innermost levels but clamps at empty.
puts inner.trim(1).path                    #=> outer
puts inner.trim(5).empty?                  #=> true
