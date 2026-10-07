# Zed

buti runs in Zed's terminal like in any other, and a few integrations make it work with the editor around it:

| | |
|---|---|
| [buti in a tab](#buti-in-a-tab) | a task opens buti as an editor tab, on a key |
| [Open files at the line](#open-files-at-the-line) | `o` opens the file at the cursor's line, without leaving buti |
| [Diffs in Zed](#diffs-in-zed) | `Z` (and `D` in Zed's terminal) opens the selected file, commit or branch in Zed's diff view |
| [Comment from Zed](#comment-from-zed) | a task leaves a review comment on the line under Zed's cursor |
| [Comments in Zed](#comments-in-zed) | the buti extension shows open review comments as diagnostics, with **Resolve** and **Dismiss** actions |

`o`, `Z` and `D` work as soon as buti runs in Zed's terminal. Seeing and leaving comments in Zed takes a one-time
[setup](#set-up).

## Set up

1. Put `buti` on your `PATH` (see the [README](../README.md) for installing it). The extension and the tasks run it
   from there.
2. Install the extension, which shows the comments:
   1. Get a checkout of buti (the extension is not in Zed's registry yet).
   2. In Zed, run **zed: install dev extension** from the command palette (`cmd-shift-p`) and pick the `editors/zed`
      folder of the checkout. Zed compiles it, which takes about half a minute the first time.
3. Add the task that leaves a comment to `~/.config/zed/tasks.json`, for every project (or `.zed/tasks.json` in one
   repository). The file is a list; create it with `[` `]` around the task if it doesn't exist:

   ```json
   [
     {
       "label": "buti: comment on this line",
       "command": "buti",
       "args": ["review", "comment", "--worktree", "--author", "user",
                "--file", "$ZED_FILE", "--line", "$ZED_ROW"],
       "cwd": "$ZED_WORKTREE_ROOT",
       "use_new_terminal": true,
       "allow_concurrent_runs": true,
       "reveal": "always",
       "hide": "on_success"
     }
   ]
   ```

4. Bind it to a key in `~/.config/zed/keymap.json`, inside its list. `cmd-alt-c` replaces Zed's own binding for that
   key, if it has one; pick another key to keep it:

   ```json
   {
     "context": "Editor",
     "bindings": { "cmd-alt-c": ["task::Spawn", { "task_name": "buti: comment on this line" }] }
   }
   ```

5. Optionally, show a comment's text at the end of its line rather than only on hover, in `~/.config/zed/settings.json`:

   ```json
   "diagnostics": { "inline": { "enabled": true } }
   ```

Then, in a GitButler repository: put the cursor on a changed line, press `cmd-alt-c`, type the comment and end it with
an empty line. It shows up in buti, and in Zed as a diagnostic within a few seconds; `cmd-.` on it resolves or
dismisses it.

### When it doesn't work

| Symptom | Cause and fix |
|---|---|
| **Failed to install dev extension: compiling Rust extension** | Zed needs Rust with `rustup` and compiles outside any project. **zed: open log** has the reason. With [mise](https://mise.jdx.dev), `No version is set for shim: rustc` means mise has no global Rust: `mise use -g rust@latest`, then install the extension again. |
| No comments show | `buti` is not on the `PATH` Zed sees, or the file's language is not in the extension's list (`editors/zed/extension.toml`). Errors from `buti lsp` are in **zed: open log**. |
| The task says a line `is not part of any change in the workspace` | Only lines in a diff can be commented on: uncommitted changes or a commit on an applied branch. |
| `Z` or `D` says Zed's command line tool was not found | buti looks for `zed` on `PATH` and in the usual install places; run **cli: install** in Zed. |

## buti in a tab

To open buti as an editor tab rather than in the terminal panel, add this task too:

```json
{
  "label": "buti",
  "command": "buti",
  "cwd": "$ZED_WORKTREE_ROOT",
  "use_new_terminal": false,
  "allow_concurrent_runs": false,
  "reveal_target": "center",
  "hide": "never"
}
```

Bind it in `keymap.json`:

```json
{
  "context": "Workspace",
  "bindings": { "cmd-alt-g": ["task::Spawn", { "task_name": "buti" }] }
}
```

## Open files at the line

`o` opens the selected file in your editor: `$VISUAL`, else `$EDITOR`, else Zed when buti runs in Zed's terminal
(`TERM_PROGRAM=zed`), else `vi`. With the details pane focused it opens at the cursor's line, otherwise at the first
change of the selected hunk. Zed, VS Code (and Cursor, Windsurf, VSCodium) and Sublime Text open next to buti, which
keeps running; a `--wait` flag in `$EDITOR`, there for `git commit`, is left out. Terminal editors (`vim`, `nvim`,
`nano`, `emacs`, `micro`, `kak`, `hx`) take over the terminal until you quit them, as before.

For a file in a commit, the line is the one in that commit; the file in your working copy may have moved on.

## Diffs in Zed

`Z` opens the selection in Zed's diff view (`zed --diff`). In Zed's terminal, `D` does too, instead of making the
details pane full screen; with nothing to diff selected, `D` still goes full screen.

- an uncommitted file or hunk, or all uncommitted changes: one diff per file, with your working copy on the right, so
  you can edit it there;
- a file in a commit, a commit, or a branch: read-only copies of the files before and after it, in one multi-file view.

The copies live in the repository's git folder (`.git/buti/diff`), where Zed's language servers find the project's
toolchain and `go.mod` or `package.json`, and never get committed; buti removes the ones older than a day. `Z` needs
Zed's CLI: the `zed` from `$EDITOR`, a `zed` on `PATH`, or else the one inside an installed Zed (`Zed.app` on macOS,
`~/.local/zed.app` on Linux), so it works in Zed's terminal without **cli: install**.

## Comment from Zed

The task from [Set up](#set-up) runs `buti review comment` on the line under the cursor: a terminal asks for the comment
(end it with an empty line), and the comment shows up in buti and, with the extension, in Zed. `--worktree` counts the
line in the file as Zed shows it; buti finds it in the uncommitted changes, or else in a commit that has it, preferring
one that changes the line. A line that is not part of any change in the workspace can't be commented on.

The task also works in a diff that `Z` or `D` opened: on a copy of a commit or a branch, the comment goes on that change
(for a branch, on its commit that has the line), on the side of the diff the copy shows.

## Comments in Zed

`buti lsp` is a language server that publishes the open review comments as diagnostics, on the lines they are on in your
working copy. They show in the editor, on hover and in **Project Diagnostics** (`cmd-shift-m`), and follow buti: a
comment left, resolved or moved in buti or by an agent shows up within a few seconds. On a comment, the code actions
(`cmd-.`) **Resolve buti comment** and **Dismiss buti comment** close it. The diffs that `Z` and `D` open show the
comments on their change too, on the side they are on.

A comment's text starts with 💬, which tells it apart from the compiler's diagnostics. Zed draws no diagnostics in its
diff views, neither the underline nor the inline text, so there a comment shows on hover only.

| Severity | Comment |
|---|---|
| Warning | starts with `[must-fix]` |
| Information | open |
| Hint | outdated, or its line is no longer in the working copy |

The extension that starts `buti lsp` is in [`editors/zed`](../editors/zed); [Set up](#set-up) installs it.

Any editor that speaks LSP can run `buti lsp` the same way; it reads the repository from the editor's workspace root.
