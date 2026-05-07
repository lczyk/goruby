# unicode edge cases: identifiers, method names, symbols

# unicode identifiers
привет = 1
метод = -> { 2 }
переменная = привет + метод.call

# unicode method names
def метод(аргумент)
  аргумент * 2
end

# unicode in strings
"こんにちは"
'привет мир'
"emoji: \u{1F600}"

# unicode in symbols
:'привет'
:"こんにちは"

# unicode in heredoc
<<~UNICODE
  こんにちは
  世界
UNICODE

# unicode escapes in strings
"\u{3042}\u{3044}\u{3046}"
"\u{41}"

# unicode identifier with method call
метод.вызов

# unicode method names with ? and ! suffixes
def готово?
  true
end

def сохранить!
end
