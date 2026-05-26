# minversion: 2.6
# Pin String#tr_s -- tr + squeeze (collapse adjacent duplicates
# in the translation set).

# tr leaves the runs intact.
puts "hello".tr("l", "*")                     #=> he**o

# tr_s squeezes the runs left by translation.
puts "hello".tr_s("l", "*")                   #=> he*o

# tr_s with range -- all chars in range translate to single target.
puts "aaabbbccc".tr_s("a-c", "*")             #=> *

# Multi-char to multi-char.
puts "hello world".tr_s("lo", "*-")           #=> he*- w-r*d
                                              # (no adjacent duplicates here)

# Vowel collapse.
puts "queueing".tr_s("aeiou", "*")            #=> q*ng

# Untranslated chars not squeezed.
puts "aabb".tr_s("b", "x")                    #=> aax
