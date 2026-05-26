# minversion: 2.6
# Pin retry inside a rescue body -- re-runs the surrounding
# begin body. Standard ruby pattern for transient errors.

attempts = 0
result = begin
  attempts += 1
  raise "transient" if attempts < 3
  "succeeded after #{attempts}"
rescue
  retry if attempts < 3
  "gave up"
end
puts result                                   #=> succeeded after 3
puts attempts                                 #=> 3

# retry inside a class method body's begin/rescue.
class Service
  def initialize
    @tries = 0
  end
  def call
    begin
      @tries += 1
      raise "flaky" if @tries < 2
      "ok"
    rescue
      retry
    end
  end
  attr_reader :tries
end

s = Service.new
puts s.call                                   #=> ok
puts s.tries                                  #=> 2

# ensure still runs after retry returns.
ensure_count = 0
done = begin
  raise "once"
rescue
  if ensure_count == 0
    ensure_count += 1
    retry rescue nil   # tries to retry, succeeds since rescue handles
  end
  "after-retry"
ensure
  ensure_count += 10
end
puts done                                     #=> after-retry
puts ensure_count                             #=> 11

# Method-body rescue with retry (no explicit begin/end).
$tries = 0
def flaky
  $tries += 1
  raise "no" if $tries < 2
  "yes"
rescue
  retry
end
puts flaky                                    #=> yes
puts $tries                                   #=> 2
