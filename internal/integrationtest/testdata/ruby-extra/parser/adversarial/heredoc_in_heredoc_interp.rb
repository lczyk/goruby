# Heredoc body containing #{} interpolation that itself contains a heredoc.
# Inner heredoc body must terminate before outer body resumes.
x = <<EOS
before #{<<INNER
inner body
INNER
} after
EOS
