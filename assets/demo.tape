Output assets/demo.gif

Set Shell "zsh"
Set FontSize 22
Set Width 1500
Set Height 900
Set TypingSpeed 75ms
Set Framerate 30
Set Padding 30

# Hidden setup: clean prompt, enable inline comments, fresh state.
Hide
Type 'export PS1="❯ "' Enter
Type "setopt interactive_comments" Enter
Type "clear" Enter
Type "rm -f ~/.config/nlci/definitions/jq.nlci.yaml" Enter
Type "clear" Enter
Show

# 1. Classic dispatch — known tool, plain English.
Type "# Translate intent into a real docker command:" Sleep 500ms Enter
Type 'nlci docker "show me running containers" --dry-run' Sleep 500ms Enter
Sleep 6s

Type "" Enter
Sleep 500ms

# 2. Tool router — no tool argument, nlci picks one.
Type "# Skip the tool name — nlci routes for you:" Sleep 500ms Enter
Type 'nlci "list my open pull requests" --dry-run' Sleep 500ms Enter
Sleep 7s

Type "" Enter
Sleep 500ms

# 3. Auto-init — first use of an undefined tool scaffolds itself.
Type "# First time using a tool? It defines itself:" Sleep 500ms Enter
Type 'nlci jq "extract .name from input.json" --dry-run' Sleep 500ms Enter
Sleep 25s

Type "" Enter
Sleep 500ms

# 4. Epilogue — the auto-init'd tool is now persistent.
Type "nlci config | tail -8" Sleep 500ms Enter
Sleep 4s

Sleep 2s
