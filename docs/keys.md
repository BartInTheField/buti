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
| `d` / `D` / `+` `-` | details pane / full screen / resize |
| `tab` | cycle focus: sidebar → lanes → details. In details: `j`/`k` hunks (scrolling through one taller than the pane first), `J`/`K` scroll, `space` marks a hunk, then `c` / `r` / `x` on hunks |
| `y` / `Y` | copy (branch name, change id, path, hunk) / pick what to copy |
| `o` / `O` | open a file in `$EDITOR` / with the default app; `o` on a branch with a pull request opens it in the browser |
| `/` / `t` | go to anything (fuzzy) / go to a branch |
| `:` / `!` | run a `but` command (the output is shown) / a shell command |
| `ctrl+p` | command palette (also holds commands without a key: **Version**, **Update buti**) |
| `j` `k` `h` `l` · `J` `K` · `g` `G` | move · next/previous branch · top/bottom |
| `esc` | back: leave the mode, clear marks, close details |
| `ctrl+r` · `q` | reload (it also polls every 3 seconds) · quit |
