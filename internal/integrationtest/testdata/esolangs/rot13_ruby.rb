# ROT13-Ruby interpreter
# source: https://esolangs.org/wiki/ROT13-Ruby
# licence: CC0 (esolangs.org wiki content)
def rot13(s)
  s.tr('A-Za-z', 'N-ZA-Mn-za-m')
end

eval rot13(ARGF.read)
