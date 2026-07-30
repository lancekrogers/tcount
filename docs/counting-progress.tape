# Live directory counting progress on a LARGE tree (multi-second run).
# From the tcount repo root after `just build-only`:
#   bash docs/seed-progress-demo.sh
#   env -u NO_COLOR -u CLICOLOR CLICOLOR_FORCE=1 FORCE_COLOR=1 vhs docs/counting-progress.tape
#
# Output: docs/counting-progress.gif

Output docs/counting-progress.gif

Set Shell "bash"
Set FontFamily "JetBrainsMono Nerd Font"
Set FontSize 16
Set Width 960
Set Height 420
Set Padding 18
Set Margin 0
Set BorderRadius 8
Set Framerate 24
Set TypingSpeed 1ms
Set Theme {"name":"tcount-progress","background":"#0d0d11","foreground":"#f8f8f2","cursor":"#bd93f9","selection":"#2a2a33","black":"#16161a","red":"#ff5f57","green":"#50fa7b","yellow":"#febc2e","blue":"#57a6ff","magenta":"#bd93f9","cyan":"#8be9fd","white":"#f8f8f2","brightBlack":"#6b7280","brightRed":"#ff6e67","brightGreen":"#69ff94","brightYellow":"#ffd23f","brightBlue":"#7ab8ff","brightMagenta":"#d0a9ff","brightCyan":"#a4f0ff","brightWhite":"#ffffff"}

Hide
Type `export PATH="$PWD/bin:$PATH"` Enter
Type `export TERM=xterm-256color COLORTERM=truecolor` Enter
Type `unset NO_COLOR CLICOLOR` Enter
Type `export CLICOLOR_FORCE=1 FORCE_COLOR=1` Enter
# ~2000 files / ~24MB — counting takes a few seconds so the progress frame is visible.
Type `bash docs/seed-progress-demo.sh /tmp/tcount-progress-demo 2000` Enter
Sleep 3s
Type `clear` Enter
Show

# Scene 1: default multi-method — watch files/chars climb for several seconds
Type `# large tree (~2000 files) — watch live progress while counting` Enter
Sleep 700ms
Type `tcount -d /tmp/tcount-progress-demo` Enter
# Hold long enough to see mid-run progress, completion, and the final table.
Sleep 8s

Type `clear` Enter
Sleep 400ms

# Scene 2: --model so labeled tokens climb live
Type `# same tree with --model gpt-5 (labeled live tokens)` Enter
Sleep 700ms
Type `tcount -d --model gpt-5 /tmp/tcount-progress-demo` Enter
Sleep 8s
