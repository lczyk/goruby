# Tomagotcha! interpreter
# source: https://esolangs.org/wiki/Tomagotcha!
# licence: CC0 (esolangs.org wiki content)

#!/usr/bin/env ruby

if (t = Time.now) - File.mtime($0) > 14400
  eval DATA.read rescue `rm #$0` and exit
  exit IO.write($0, `head -n #{DATA.lineno - 1} #$0`)
end

`echo -n '#{eval(t.strftime '%H+%M+%S').chr}' >> #$0`

__END__
