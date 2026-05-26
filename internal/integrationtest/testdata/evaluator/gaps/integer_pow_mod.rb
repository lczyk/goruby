# Integer#pow(exp, mod) computes (self ** exp) mod mod, but without
# the intermediate huge integer. Critical for modular arithmetic /
# crypto-style code.
puts 2.pow(10, 1000)                      #=> 24
puts 3.pow(7, 100)                        #=> 87
puts 5.pow(0, 13)                         #=> 1
puts 2.pow(10)                            #=> 1024
