# Live directory counting progress (stderr frame → lipgloss report).
# From the tcount repo root after `just build-only`:
#   bash docs/seed-progress-demo.sh
#   vhs docs/counting-progress.tape
#
# Output: docs/counting-progress.gif

Output docs/counting-progress.gif

Set Shell "bash"
Set FontFamily "JetBrainsMono Nerd Font"
Set FontSize 16
Set Width 920
Set Height 520
Set Padding 18
Set Margin 0
Set BorderRadius 8
Set Framerate 24
Set TypingSpeed 1ms
Set Theme {"name":"tcount-progress","background":"#0d0d11","foreground":"#f8f8f2","cursor":"#bd93f9","selection":"#2a2a33","black":"#16161a","red":"#ff5f57","green":"#50fa7b","yellow":"#febc2e","blue":"#57a6ff","magenta":"#bd93f9","cyan":"#8be9fd","white":"#f8f8f2","brightBlack":"#6b7280","brightRed":"#ff6e67","brightGreen":"#69ff94","brightYellow":"#ffd23f","brightBlue":"#7ab8ff","brightMagenta":"#d0a9ff","brightCyan":"#a4f0ff","brightWhite":"#ffffff"}

Hide
Type `export PATH="$PWD/bin:$PATH"` Enter
Type `export TERM=xterm-256color COLORTERM=truecolor` Enter
Type `unset NO_COLOR` Enter
Type `bash docs/seed-progress-demo.sh` Enter
Sleep 1s
Type `clear` Enter
Show

Type `# recursive directory count — live progress on a TTY` Enter
Sleep 500ms
Type `tcount -d /tmp/tcount-progress-demo` Enter
Sleep 5s

Type `clear` Enter
Sleep 300ms

Type `# with --model: labeled live tokens while counting` Enter
Sleep 500ms
Type `tcount -d --model gpt-5 /tmp/tcount-progress-demo` Enter
Sleep 5s
