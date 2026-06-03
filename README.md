# tick

Jot down the next open steps in your projects and cross them off — with a
satisfying confetti pop when you do.

![tick crossing a step off, confetti raining](assets/screenshot.png)

`tick` works two ways from one binary:

- **Per-project** — run it inside a repo to manage that project's steps.
- **Globally** — an overview of every tracked project; pick one, or track the
  current directory right from the menu.

Steps live in a single global store (`~/.config/tick/store.json`, honoring
`XDG_CONFIG_HOME`), keyed by absolute project path. Nothing is written into your repos.

## Install

```sh
go install .        # builds `tick` into $GOBIN / $GOPATH/bin
# or:
go build -o tick . && mv tick ~/bin/   # ensure ~/bin is on PATH
```

Requires Go 1.26+.

## Interactive (TUI)

```sh
tick        # tracked dir -> that project's steps; untracked -> global overview
tick ui     # always open the global overview
```

Steps are a **priority list, not a checklist**: open steps are numbered `1. 2. 3.`
top-to-bottom, and you order them yourself.

**Steps screen** — `x`/`enter` cross off (confetti!) or reopen a done step · `a` add ·
`e` edit · `i` mark important (adds a `!`, keeps its position) · `t` bump to the top ·
`⇧↑`/`⇧↓` reorder · `d` delete · `u` undo a delete · `y` copy the project's path ·
`h` show/hide done · `esc`/`g` overview · `q` quit.

**Overview** — `↑`/`↓` or `j`/`k` move · `enter` open (or **+ Track current dir**) ·
`g` assign the project to a group (type a name, blank clears) · `y` copy its path ·
`d` untrack a project (with confirm) · `q` quit. Projects are shown in sections by
group (e.g. `work`, `personal`), with ungrouped ones last.

**Editing prompts** (add / edit / group) support a real cursor: `←`/`→` move,
`⌥←`/`⌥→` jump by word, `Home`/`End` (or `Ctrl-A`/`Ctrl-E`) jump to the ends,
`Delete` removes forward, and `⌘V` pastes. (Word-jump with `⌥`+arrow depends on your
terminal sending it.)

If the current directory isn't tracked yet, bare `tick` drops you on the overview
with a one-key **+ Track current dir** row; selecting it starts tracking and jumps
straight into that project's steps.

## Scriptable

```sh
tick add "wire up confetti"     # append a step (creates the project on first add)
tick ls [--all]                 # list open steps (--all includes done); important ones show a !
tick done 1 3                   # cross off step(s) by number
tick undone 2                   # reopen
tick rm 4                       # delete a step
tick edit 2 "new text"          # change a step's text
tick clear [--all-projects]     # drop done steps
tick projects                   # all tracked projects + open counts
tick untrack [-y]               # stop tracking the current project
```

Most commands accept `-p <path>` to target another project. Step numbers are the
ones shown by `tick ls`.

## How it works

- **Store** (`internal/store`) — one JSON file; every mutation is a locked
  read-modify-write (`flock`) with an atomic temp-file rename, so concurrent
  `tick` invocations never corrupt or lose data. Project keys are symlink-resolved
  absolute paths, found by walking up from the CWD to the nearest `.git`.
- **TUI** (`internal/tui`) — Bubble Tea v2 + Lip Gloss, hand-rolled list renderer.
  Cross-off is *animate-then-remove*: the step is persisted done immediately, the
  row stays visibly struck while the confetti burst (a `harmonica` particle
  simulation) plays over it, then it drops from the open list.

## Develop

```sh
go test ./...          # unit tests (store, paths, confetti, TUI model)
go test -race ./...
```
