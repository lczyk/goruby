# skip-evaluator: needs def
$count = 0

def bump
  $count = $count + 1
end

bump
bump
bump
puts $count   #=> 3
