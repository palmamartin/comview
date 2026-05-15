# comview

The finest diff viewer ever compressed into a small terminal program. It reads a unified diff from stdin, renders it beautifully, and lets you review code without becoming a web browser.

![comview screenshot](screenshot.png)

## Install

```sh
git clone https://github.com/rockorager/comview.git
cd comview
make install
```

To install somewhere else:

```sh
make PREFIX=$HOME/.local install
```

## Usage

```sh
git diff | comview
git show | comview
gh pr diff 123 | comview
```

Comments are saved to `.comview/comments.json`.

## Keybinds

| Key | Action |
| --- | --- |
| `j`/`k`, arrows | Move |
| `h`/`l` | Move horizontally |
| `gg` / `G` | Top / bottom |
| `Ctrl-d` / `Ctrl-u` | Half-page down / up |
| `J` / `K` | Next / previous commit |
| `]c` / `[c` | Next / previous change |
| `]n` / `[n` | Next / previous note |
| `s` | Toggle side-by-side view |
| `<space>e` | Find file in diff |
| `/` | Search |
| `n` / `N` | Next / previous search result |
| `o` | Open cursor location in editor |
| `v` / `V` | Visual / visual-line selection |
| `iw`, `aw`, `i{`, `a"`, etc. | Text objects, naturally flawless |
| `y` | Copy selection |
| `i` or `I` | Add/edit comment |
| `x` / `dd` | Delete note under cursor |
| `:w` | Save comments |
| `:q` / `:q!` | Quit / force quit |
| `?` | Show this help |
| `Esc` | Cancel |

That is all. It is enough.
