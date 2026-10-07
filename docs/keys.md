# Keys

`?` in buti shows the same list, searchable.

| Key | Action |
|---|---|
| `c` | commit the selection, marks, or everything (in the target picker: `a` above/below, `b` new branch here, `e` empty message) |
| `r` / `R` | squash, amend or uncommit into a target (`u` keeps the target message) / amend all changes into the selection |
| `m` / `p` | move / cherry-pick commits (`a` above/below); move a branch to stack or unstack it |
| `enter` | reword a commit or rename a branch; open a file's diff; fold a folder |
| `M` | reword in `$EDITOR` |
| `n` | insert an empty commit |
| `A` | absorb changes into the commits they belong to |
| `x` | discard, after confirming (undo with `u`) |
| `e` | resolve a conflicted commit (`✗`) in edit mode; in edit mode: save and exit (`x` cancels, `o` opens the conflicted files, `enter` the selected one) |
| `b` / `B` | new branch: stacked on the selected branch, or a new lane / below the selected branch |
| `P` / `N` | push the branch / open a pull request |
| `a` / `S` | apply a branch (picker) / unapply the stack |
| `L` | pull upstream changes |
| `u` / `U` / `H` | undo / redo / operation history (restore any snapshot) |
| `space` | mark (on **Unstaged**, marks every file) |
| `f` / `F` | files in the commit / in every commit |
| `d` / `D` / `+` `-` | details pane / full screen (in Zed's terminal: the [diff in Zed](zed.md#diffs-in-zed)) / resize (the pane, or full screen the file tree; the dividers can also be dragged) |
| `T` | in full-screen details: show / hide the file tree (`+` / `-` or dragging its divider resizes it) |
| `tab` | cycle focus: sidebar → lanes → details. In details: `j`/`k` move the line cursor (click a line to put it there), `J`/`K` scroll, `space` marks the cursor's hunk, then `c` / `r` / `x` on hunks. In full-screen details: `tab` (or `h` / `l`) moves between the file tree and the diff; in the tree `j`/`k` jump the diff to a file and `enter` folds a folder |
| `[` / `]` | in details: previous / next hunk |
| `v` | in details: select a range of lines from the cursor (or shift-click); moving extends it, `v` or `esc` cancels |
| `C` | in details: comment on the cursor's line or range (`ctrl+s` saves, `enter` is a new line) |
| `e` / `d` / `x` | on a comment in details: edit / delete (after confirming) / resolve or reopen it |
| `z` | show / hide resolved comments; **Review comments…** in the palette lists the open ones and jumps to them |
| `y` / `Y` | copy (branch name, change id, path, hunk) / pick what to copy |
| `o` / `O` | open a file in your editor, at the cursor's line ([Zed](zed.md#open-files-at-the-line)) / with the default app; `o` on a branch with a pull request opens it in the browser |
| `Z` | open the diff of a file, hunk, commit, branch or all uncommitted changes in [Zed](zed.md#diffs-in-zed) |
| `/` / `t` | go to anything (fuzzy) / go to a branch |
| `:` / `!` | run a `but` command (the output is shown) / a shell command |
| `ctrl+p` | command palette (also holds commands without a key: **Version**, **Update buti**) |
| `j` `k` `h` `l` · `J` `K` · `g` `G` | move · next/previous branch · top/bottom |
| `esc` | back: leave the mode, clear marks, close details |
| `ctrl+r` · `q` | reload and sync pull requests from the forge (it also polls every 3 seconds, without the sync) · quit |
