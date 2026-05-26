# protected methods reject explicit-receiver calls from outside the
# class hierarchy, but allow them between instances of the same class
# (or subclass). Differs from private: private rejects any explicit
# receiver, even self/same-class peer.
class Account
  def initialize(bal); @bal = bal; end
  def richer_than?(other)
    balance > other.balance
  end
  protected
  def balance; @bal; end
end

a = Account.new(100)
b = Account.new(50)
puts a.richer_than?(b)                    #=> true

begin
  a.balance
rescue NoMethodError
  puts "balance rejected"                 #=> balance rejected
end
