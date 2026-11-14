# Record with: .amp/buti record .amp/tapes/example.tape
# Copy this tape for a new feature. Paths are relative to .amp/in/artifacts. Reference: https://github.com/charmbracelet/vhs
Output example.mp4

Set Width 1400
Set Height 800
Set FontSize 14
Set Padding 10

# Start buti off screen, so the video opens on the workspace.
Hide
Type "buti && exit"
Enter
Sleep 2s
Show

Sleep 1s
Screenshot example-workspace.png

# Walk the Unstaged tree and open a file's diff.
Down@300ms 5
Enter
Sleep 1s
Screenshot example-details.png

Type "?"
Sleep 1s
Screenshot example-help.png
Escape
Sleep 500ms

Type "q"
Sleep 500ms
