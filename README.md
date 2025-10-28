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
 * Custom [Styling](#Styling) with GTK+ CSS
 * Image support (experimental, can be enabled through config `image-support = true`)
   - Currently copying images is only possible between file managers until i figure out how to elegantly implement multiple mime types to wl-clipboard

## Keybinds

- `j`, `↓`, `tab` - move a line down
- `k`, `↑`, `shift+tab` - move a line up
- `ctrl+u` - move 5 lines up
- `ctrl+d` - move 5 lines down
- `i`, `/` - Focus search bar
- `g` - go to top
- `G` - go to bottom
- `delete`, `D` - delete selected item from history
- `esc` - close window or focus history list if search bar is focused
- `ctrl+c` - close application (this would also stop monitoring the clipboard)


## CLI

Currently the following arguments are available:

```
--clear-history                 Clears the history file
--system-theme=true|false       Whether to respect your system's gtk theme (default: false)
--max-entries=100               Maximum amount of clipboard entries the history should hold (default: 100)
--layer-shell=true|false        Whether to use the gtk-layer-shell instead of a normal window (default: true)
--silent=true|false             Starts bbclip silently in the background (default: false)
--icons=true|false              Whether to display row icons (default: false)
--text-preview-length=100       The Length of the text preview before it's truncated
--image-support=true|false      Whether copying should be possible (default: false)
--image-preview=true|false      Whether to show a preview of the copied image (default: true)
--image-height=50               The height of the preview image if image-support is enabled (default: 50)
```

You can write the same flags (without the double dashes) in `~/.config/bbclip/config` to make it persistent.


## Styling

Create a `style.css` in `~/.config/bbclip/` and use the following classes:

- `.popup-wrapper {}` - The main popup window (GtkBox)
- `.search {}` - The search input (GtkEntry)
- `.entries-list {}` - The history items list (GtkListBox)
- `.entries-list-row {}` - A history item row (GtkListBoxRow)


---
> [!NOTE]
> I recommended to start `bbclip` with your system via `bbclip --silent` otherwise it would just start monitoring your clipboard after you first launched the app.
> This basically just spawns `bbclip` silently in the background.
---
> [!NOTE]
> bbclip doesn't really work on Plasma 6 due to the weird focus stealing thing with overlays/layer shells (You guys have Klipper anyway) but it should probably work on any wayland compositors like Niri (tested) or Hyprland.
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
