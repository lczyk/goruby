puts Encoding.list.size > 0
puts Encoding.name_list.include?("UTF-8")
puts Encoding::UTF_8.name
puts Encoding::ASCII_8BIT.name
puts Encoding.find("utf-8").name
puts Encoding.compatible?("a", "b").inspect
