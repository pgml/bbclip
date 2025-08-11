package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/dlasky/gotk3-layershell/layershell"
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

const socketPath = "/tmp/bbclip.sock"

//go:embed style.css
var defaultStyle []byte

var (
	version string
	dev     string
	commit  string

	ShowVersion  = flag.Bool("version", false, "Shows the version")
	ClearHistory = flag.Bool("clear-history", false, "Clears the history")
	SystemTheme  = flag.Bool("system-theme", false, "Uses the system gtk theme")
	MaxEntries   = flag.Int("max-entries", 100, "Maximum amount of clipboard entries the history should hold")
	LayerShell   = flag.Bool("layer-shell", true, "Use layer shell instead of window")
)

type BBClip struct {
	// history is the clipboard history
	history *History

	// window is the gtk window
	window *gtk.Window

	// popupWrapper is the window wrapper that contains the search field
	// and the history entries
	popupWrapper *gtk.Box

	// search is the search input field
	search *gtk.Entry

	// entriesListWrapper is the scrolled window view containing the
	// history entries
	entriesListWrapper *gtk.ScrolledWindow

	// entriesList is the history entries list view
	entriesList *gtk.ListBox

	// cssProvider is the gtk css provider
	cssProvider *gtk.CssProvider

	visTime time.Time
}

func main() {
	flag.Parse()

	if *ShowVersion {
		printVersion()
		return
	}

	gtk.Init(nil)

	if tryConnectSocket() {
		fmt.Println("Another instance already running. Exiting.")
		return
	}

	bbclip := BBClip{history: &History{}}
	bbclip.buildUi()
	bbclip.listenSocket()
	bbclip.window.ShowAll()

	gtk.Main()
}

// buildUi builds the main ui, applies css style and populates the
// clipboard history.
func (b *BBClip) buildUi() {
	b.history = NewHistory()
	b.history.Init()

	var err error

	b.window, err = gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		log.Fatal("Unable to create window:", err)
	}

	if *LayerShell {
		// @todo make config option: `use-layer-shell = true`
		layershell.InitForWindow(b.window)
		layershell.SetNamespace(b.window, "gtk-layer-shell")
		layershell.SetAnchor(b.window, layershell.LAYER_SHELL_EDGE_TOP, false)
		layershell.SetLayer(b.window, layershell.LAYER_SHELL_LAYER_OVERLAY)
		layershell.SetMargin(b.window, layershell.LAYER_SHELL_EDGE_TOP, 0)
		layershell.SetMargin(b.window, layershell.LAYER_SHELL_EDGE_LEFT, 0)
		layershell.SetMargin(b.window, layershell.LAYER_SHELL_EDGE_RIGHT, 0)
		layershell.SetExclusiveZone(b.window, 30)
		layershell.SetKeyboardMode(b.window, layershell.LAYER_SHELL_KEYBOARD_MODE_EXCLUSIVE)
	}

	b.popupWrapper, _ = gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 8)

	b.search, _ = gtk.EntryNew()
	b.search.SetCanFocus(false)
	b.search.SetPlaceholderText("Search")
	b.search.Connect("button-press-event", b.onButtonPress)
	b.search.Connect("key-release-event", b.onKeyRelease)

	b.entriesList, _ = gtk.ListBoxNew()
	b.entriesList.SetSelectionMode(gtk.SELECTION_SINGLE)
	b.entriesList.SetMarginBottom(6)
	b.entriesList.Connect("row-activated", b.onRowActivated)

	b.entriesListWrapper, _ = gtk.ScrolledWindowNew(nil, nil)
	b.entriesListWrapper.SetSizeRequest(350, 450)
	b.entriesListWrapper.SetOverlayScrolling(true)
	b.entriesListWrapper.Add(b.entriesList)

	b.refreshEntryList()

	b.window.SetTitle("bellbird clipboard")
	b.window.SetDecorated(false)
	b.window.SetDefaultSize(400, 400)
	b.window.SetResizable(false)
	b.window.SetAppPaintable(true)
	b.window.Connect("key-press-event", b.onKeyPress)
	b.window.Connect("focus-out-event", b.onFocusOut)

	b.applyStyles()

	b.popupWrapper.PackStart(b.search, true, true, 0)
	b.popupWrapper.PackStart(b.entriesListWrapper, true, true, 0)
	b.window.Add(b.popupWrapper)
}

// listenSocket sets up a Unix domain socket server to listen for incoming commands.
// It removes any existing socket file at the defined path, then listens asynchronously.
// When a "SHOW" command is received, it triggers the GUI to refresh and
// bring the window to the foreground.
func (b *BBClip) listenSocket() {
	// Remove any existing socket file to avoid "address already in use" error.
	os.Remove(socketPath)

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		panic(err)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				fmt.Println("accept err:", err)
				continue
			}

			buf := make([]byte, 16)
			n, _ := conn.Read(buf)

			if string(buf[:n]) == "SHOW\n" {
				glib.IdleAddPriority(glib.PRIORITY_HIGH_IDLE, func() {
					b.refreshEntryList()
					b.window.ShowAll()
					b.window.Present()
					b.window.Deiconify()
					b.goToTop()

					b.visTime = time.Now()
				})
			}
			conn.Close()
		}
	}()
}

func (b *BBClip) handleKeyEvents(key *gdk.EventKey) bool {
	name := gdk.KeyValName(key.KeyVal())

	switch name {
	case "Escape":
		if b.search.HasFocus() {
			b.focusEntryList()
			return true
		} else {
			b.window.Hide()
		}

	case "Return":
		if b.entriesList != nil {
			row := b.entriesList.GetSelectedRow()
			b.selectAndHide(row)
		}

	case "k", "Up":
		b.rowUp()

	case "j", "Down":
		b.rowDown()

	case "i", "slash":
		if !b.search.HasFocus() {
			b.search.SetCanFocus(true)
			b.search.GrabFocus()
			return true
		}
	}

	return false
}

// searchAndFocus hides or shows clipboard entries depending
// on the given search query and automatically selects the first result.
// If ignoreCase is true the search is case insensitive.
func (b *BBClip) searchAndFocus(query string, ignoreCase bool) {
	b.entriesList.GetChildren().Foreach(func(item any) {
		widget := item.(*gtk.Widget)

		row := gtk.ListBoxRow{
			Bin: gtk.Bin{
				Container: gtk.Container{
					Widget: *widget,
				},
			},
		}

		child, _ := row.GetChild()
		rowContent, _ := child.(*gtk.Label).GetName()

		if ignoreCase {
			rowContent = strings.ToLower(rowContent)
			query = strings.ToLower(query)
		}

		if strings.Contains(rowContent, query) {
			row.Show()
		} else {
			row.Hide()
		}
	})

	b.goToTop()
}

// focusEntryList focues the clipboard history and removes focus
// from the search fields.
func (b *BBClip) focusEntryList() {
	b.entriesList.SetCanFocus(true)
	b.entriesList.GrabFocus()
	b.search.SetCanFocus(false)
}

// selectAndHide copies the selected row's content to the clipboard,
// moves the row to the first position and hides the window afterwards.
func (b *BBClip) selectAndHide(row *gtk.ListBoxRow) {
	if row == nil {
		b.window.Hide()
		return
	}

	child, _ := row.GetChild()
	label, err := gtk.WidgetToLabel(child.ToWidget())

	if err == nil {
		text, _ := label.GetName()

		// Move the selected row to the first position of the history list
		if ok, index := b.history.contains(text); ok {
			entries := b.history.entries
			b.history.entries = slices.Delete(entries, index, index+1)
		}

		b.history.WriteToClipboard(text)
		b.window.Hide()
	}
}

// rowUp moves the selection one row up and repositions the view if needed
func (b *BBClip) rowUp() {
	if b.search.HasFocus() {
		return
	}

	selectedRow := b.entriesList.GetSelectedRow()
	if selectedRow == nil {
		return
	}

	index := selectedRow.GetIndex()

	// look for visible lines only
	for prevIndex := index - 1; prevIndex >= 0; prevIndex-- {
		prevRow := b.entriesList.GetRowAtIndex(prevIndex)
		if prevRow.IsVisible() {
			b.entriesList.SelectRow(prevRow)
			b.repositionView()
			break
		}
	}
}

// rowDown moves the selection one row down and repositions the view if needed
func (b *BBClip) rowDown() {
	if b.search.HasFocus() {
		return
	}

	selectedRow := b.entriesList.GetSelectedRow()
	if selectedRow == nil {
		return
	}

	index := selectedRow.GetIndex()
	rowCount := int(b.entriesList.GetChildren().Length())

	// look for visible lines only
	for nextIndex := index + 1; nextIndex < rowCount; nextIndex++ {
		nextRow := b.entriesList.GetRowAtIndex(nextIndex)
		if nextRow.IsVisible() {
			b.entriesList.SelectRow(nextRow)
			b.repositionView()
			break
		}
	}
}

// goToTop moves the selection to the first row of the list and
// repositions the view
func (b *BBClip) goToTop() {
	rowCount := int(b.entriesList.GetChildren().Length())

	for nextIndex := range rowCount {
		nextRow := b.entriesList.GetRowAtIndex(nextIndex)

		if nextRow.IsVisible() {
			b.entriesList.SelectRow(nextRow)
			b.repositionView()
			return
		}
	}
}

//func (b *BBClip) goToBottom() {
//	rowCount := int(b.entriesList.GetChildren().Length())
//	index := b.entriesList.GetRowAtIndex(rowCount)
//	b.entriesList.SelectRow(index)
//	b.repositionView()
//}

// repositionView adjusts the view to the selected row
func (b *BBClip) repositionView() {
	alloc := b.entriesList.GetSelectedRow().GetAllocation()
	vadj := b.entriesListWrapper.GetVAdjustment()
	upper := vadj.GetValue() + vadj.GetPageSize()

	if float64(alloc.GetY()) < vadj.GetValue() {
		vadj.SetValue(float64(alloc.GetY()))
	}

	if float64(alloc.GetY()+alloc.GetHeight()) > upper {
		vadj.SetValue(
			float64(alloc.GetY()+alloc.GetHeight()-int(vadj.GetPageSize())) + 20,
		)
	}
}

// refreshEntryList fetches the latest clipboard entries from the history,
// rebuilds the lipboard history list and automatically sets focus
// to the history list
func (b *BBClip) refreshEntryList() {
	b.history.mu.RLock()
	defer b.history.mu.RUnlock()

	if b.entriesList == nil {
		return
	}

	children := b.entriesList.GetChildren()
	for e := children; e != nil; e = e.Next() {
		child := e.Data().(*gtk.Widget)
		b.entriesList.Remove(child)
	}

	entryIndex := 0
	entries := b.history.entries

	for i := len(entries) - 1; i >= 0; i-- {
		// truncate the preview
		name := strings.ReplaceAll(entries[i], "\n", " ")
		name = TruncateText(name, 42)

		// the real clipboard content
		text := entries[i]

		label, _ := gtk.LabelNew(name)
		// set the real content as the name
		// there's probably a better way to do this.
		label.SetName(text)
		label.SetMarginTop(6)
		label.SetMarginBottom(6)
		label.SetXAlign(0)

		row, _ := gtk.ListBoxRowNew()
		row.Add(label)
		row.ShowAll()

		b.addContextClass(row.ToWidget(), "entries-list-row")
		b.entriesList.Add(row)

		entryIndex++
	}

	b.entriesList.GrabFocus()
	b.search.SetCanFocus(false)
	b.search.SetText("")
}

func (b *BBClip) applyStyles() {
	var err error
	b.cssProvider, err = gtk.CssProviderNew()
	if err != nil {
		println("Failed to load CSS")
	}

	if err := b.cssProvider.LoadFromData(string(defaultStyle)); err != nil {
		log.Fatal("Unabled to load CSS data:", err)
	}

	screen, _ := gdk.ScreenGetDefault()
	gtk.AddProviderForScreen(
		screen,
		b.cssProvider,
		gtk.STYLE_PROVIDER_PRIORITY_APPLICATION,
	)

	if !*SystemTheme {
		b.addContextClass(&b.window.Widget, "bbclip")
	}

	b.addContextClass(&b.search.Widget, "search")
	b.addContextClass(&b.entriesListWrapper.Widget, "entries-list-wrapper")
	b.addContextClass(&b.entriesList.Widget, "entries-list")
	b.addContextClass(&b.popupWrapper.Widget, "popup-wrapper")
}

func (b *BBClip) addContextClass(widget *gtk.Widget, className string) {
	if sctx, err := widget.GetStyleContext(); err == nil {
		sctx.AddClass(className)
	}
}

func (b *BBClip) onKeyPress(_ *gtk.Window, ev *gdk.Event) bool {
	return b.handleKeyEvents(gdk.EventKeyNewFromEvent(ev))
}

func (b *BBClip) onFocusOut(win *gtk.Window, _ *gdk.Event) {
	since := time.Since(b.visTime)

	if since > 100*time.Millisecond {
		// @todo config option close-on-blur = false
		win.Hide()
		fmt.Println("Window lost focus")
	}
}

func (b *BBClip) onRowActivated(_ *gtk.ListBox, row *gtk.ListBoxRow) {
	b.selectAndHide(row)
}

func (b *BBClip) onButtonPress(entry *gtk.Entry, ev *gdk.Event) bool {
	entry.SetCanFocus(true)
	entry.GrabFocus()
	return false
}

func (b *BBClip) onKeyRelease(entry *gtk.Entry, ev *gdk.Event) bool {
	searchQuery, _ := entry.GetText()
	// @todo make ignoreCase a config option `ignore-case = true`
	b.searchAndFocus(searchQuery, true)
	return false
}

// tryConnectSocket attempts to connect to the Unix domain socket
// server and send a "SHOW" command.
// Returns true if the connection and write succeed, false otherwise.
func tryConnectSocket() bool {
	conn, err := net.Dial("unix", socketPath)

	if err != nil {
		fmt.Println(err)
		return false
	}

	defer conn.Close()
	conn.Write([]byte("SHOW\n"))

	return true
}

// TruncateText shortens the given text to fit within maxWidth.
// If the text exceeds maxWidth, it appends "..." (if possible).
func TruncateText(text string, maxWidth int) string {
	if len(text) > maxWidth {
		if maxWidth > 3 {
			return text[:maxWidth-3] + "..."
		}
		return text[:maxWidth] // No space for "..."
	}
	return text
}

func Clamp(v, lower, upper int) int {
	return min(max(v, lower), upper)
}

func printVersion() {
	if dev != "" {
		fmt.Printf("%s %s-dev.%s\n", "bbclip", version, commit)
	}

	if commit != "" && dev == "" {
		fmt.Printf("%s %s (%s)\n", "bbclip", version, commit)
	}
}
