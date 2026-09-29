# buti brand

The tagline, three lines, one strong word each:

> **Parallel** agentic workflow
> **Code review** your agent can act on
> On top of **GitButler**

The pitch in a sentence: buti is a terminal workspace for running several coding agents in parallel, one branch
each, with review comments on any diff that the agent reads, fixes and resolves. It runs on top of GitButler, which
does the branching; buti is its own thing.

Order matters: the workflow first, the review loop second, GitButler last. GitButler is the foundation, not the
headline, so it never leads a sentence about buti. Write "on top of GitButler", never "a GitButler client".

Set the lines as three rows. **Parallel** and **GitButler** in Paper, **Code review** in Mint; the rest in Smoke.

## Voice

Plain, quick, concrete. Verbs first: commit, squash, move, stack, review. No "seamless", no "powerful", no
exclamation marks. Every claim is something the screen shows a second later. The terminal is not a limitation to
apologise for; it is the speed.

## Colors

The palette is the UI's own, so marketing and product look the same.

| Name    | Hex       | Use                                                |
|---------|-----------|----------------------------------------------------|
| Ink     | `#171717` | backgrounds                                        |
| Paper   | `#ececec` | primary text                                       |
| Violet  | `#7c5cff` | the brand accent: selection, targets, the logo     |
| Mint    | `#2dd4bf` | pushed, done, the agent                            |
| Amber   | `#facc15` | local, unpushed, in progress                       |
| Green   | `#4ade80` | success toasts                                     |
| Smoke   | `#8a8a8a` | secondary text                                     |
| Line    | `#2e2e2e` | rules and card borders                             |

Violet on Ink is the lockup. Mint and Amber are used only as small accents (one dot, one word), never as
backgrounds.

## Type

- **JetBrains Mono** (Bold / ExtraBold) for the wordmark, code and anything that is a key or a command.
- **Inter** for prose and captions.

The wordmark is `buti` set in JetBrains Mono ExtraBold, lowercase, tracking -0.04em. It is never capitalised.

## Logo

- `mark.svg`: the mark alone. Three lanes, as in the workspace, with the middle one carrying a commit. Rounded
  square in Violet with the lanes in Paper; the commit dot is Mint.
- `logo.svg`: the mark with the wordmark, for banners and the README.
- `logo-dark-text.svg`: the wordmark in Ink, for light backgrounds.

Keep clear space of one lane's width around the mark. The mark works down to 16px (the favicon): at that size the
dot is dropped.

## Social

- Avatar: the mark on Ink, 1000×1000.
- Header: mark, wordmark and tagline on Ink, 1500×500.

## Suggested post

> Parallel agentic workflow. Code review your agent can act on. On top of GitButler.
>
> buti is a terminal workspace for running coding agents in parallel, a branch each, with lanes for your stacks and
> a diff pane. Leave review comments on any diff, run /buti-resolve, and the agent fixes each one and marks it
> resolved.
>
> Free, MIT. Install in one line: github.com/BartInTheField/buti
