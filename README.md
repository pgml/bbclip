# bbclip

A simple clipboard history manager that I made because i missed klipper after I
moved from Plasma.
I'm also too lazy to hook something into a launcher, I wanted something that just works after installing.

[bbclip-demo.webm](https://github.com/user-attachments/assets/e032fcac-f22a-4a0e-8bd5-056913e6f7bb)

## Features

Not many

 * Persistent clipboard
 * Filter/search in clipboard content
 * Basic vim bindings so you don't have to touch your mouse ever again

## Keybinds

- `j`, `↓`, `tab` - move a line down
- `k`, `↑`, `shift+tab` - move a line up
- `ctrl+u` - move 5 lines up
- `ctrl+d` - move 5 lines down
- `i`, `/` - Focus search bar
- `delete` - delete selected item from history
- `esc` - close window or focus history list if searchbar is focused
- `ctrl+c` - close application (this would also stop monitoring the clipboard)


## CLI

Currently the following arguments are available:

```
--clear-history                 Clears the history file
--system-theme=true|false       Whether to respect your system's gtk theme (default: false)
--max-entries=100               Maximum amount of clipboard entries the history should hold (default: 100)
--layer-shell=true|false        Whether to use the gtk-layer-shell instead of a normal window (default: true)
--silent=true|false             Starts bbclip silently in the background (default: false)
```

---
> [!NOTE]
> I recommended to start `bbclip` with your system via `bbclip --silent` otherwise it would just start monitoring your clipboard after you first launched the app.
> This basically just spawns `bbclip` silently in the background.
---


## Requirements

 * wl-clipboard
 * libgtk-3-0
 * libglib2.0-0

## Build dependencies

 * Go >= 1.20
 * build-essential
 * libgtk-3-dev
 * libglib2.0-dev
 * libgtk-layer-shell-dev
 * gcc
 * pkg-config

## Plans

* [ ] Config file
* [ ] Custom styling
