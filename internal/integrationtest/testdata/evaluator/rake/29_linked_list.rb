# minversion: 2.6
# Exercise Rake::LinkedList directly. Used inside rake for
# InvocationChain and a few other places. Tests cons/make/each/to_s.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake/linked_list"

# Empty list.
empty = Rake::LinkedList.empty
puts empty.empty?                  #=> true
puts empty.to_s                    #=> LL()

# Built from .make in head-first order.
list = Rake::LinkedList.make("a", "b", "c")
puts list.empty?                   #=> false
puts list.head                     #=> a
puts list.tail.head                #=> b
puts list.to_s                     #=> LL(a, b, c)

# conj pushes onto the head.
list2 = list.conj("z")
puts list2.head                    #=> z
puts list2.to_s                    #=> LL(z, a, b, c)

# Structural equality.
other = Rake::LinkedList.make("a", "b", "c")
puts list == other                 #=> true
puts list == list2                 #=> false

# each yields head-to-tail.
seen = []
list.each { |x| seen << x }
puts seen.join(",")                #=> a,b,c
