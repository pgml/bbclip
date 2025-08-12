# bbclip

A simple clipboard history manager that I made because i missed klipper after I 
moved from Plasma.
I'm also too lazy to hook something into a launcher, I wanted something that just works after installing.

<img width="960" height="540" alt="Screenshot from 2025-08-12 23-04-54" src="https://github.com/user-attachments/assets/aed3650f-c1b0-49fe-989a-b7ee80121a0a" />


## Features

Not many

 * Persistent clipboard
 * Filter/search in clipboard content
 * basic vim bindings

<img width="408" height="616" alt="Screenshot from 2025-08-12 18-10-36" src="https://github.com/user-attachments/assets/0276891b-e07b-44e6-9221-b68765fe0544" />
<img width="408" height="616" alt="Screenshot from 2025-08-12 18-11-21" src="https://github.com/user-attachments/assets/b0a44997-0949-4ffc-a16f-7e358cbf3e29" />

   

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




## Dependencies

 * wl-clipboard
 * libgtk-3-dev
 * libglib2.0-dev
 * libgtk-layer-shell-dev
 * gcc

## Plans

* [ ] Config file
* [ ] Custom styling
