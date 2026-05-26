# Object#send bypasses visibility (private + protected reachable).
# Object#public_send respects visibility -- private call raises
# NoMethodError, public call succeeds.
class Vault
  def open_pub
    "public open"
  end
  private
  def secret
    "buried"
  end
end

v = Vault.new
puts v.send(:secret)                      #=> buried
puts v.public_send(:open_pub)             #=> public open

begin
  v.public_send(:secret)
rescue NoMethodError
  puts "public_send rejected secret"      #=> public_send rejected secret
end
