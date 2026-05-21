# minversion: 3.0
val = {name: "x", age: 7}
case val
in {name: String => n, age: Integer => a}
  puts "#{n}/#{a}"
end

case [1, 2, 3]
in [1, *rest]
  puts rest.inspect
end
