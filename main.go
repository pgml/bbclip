package main

import (
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"net"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/dlasky/gotk3-layershell/layershell"
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
	"github.com/gotk3/gotk3/pango"
)

const (
	socketPath      = "/tmp/bbclip.sock"
	userCssFileName = "style.css"
	initialItems    = 20
	defaultWidth    = 350
	defaultHeight   = 450
)

//go:embed style.css
var defaultStyle []byte

var (
	version string
	dev     string
	commit  string

	flagShowVersion       = flag.Bool("version", false, "Shows the version")
	flagClearHistory      = flag.Bool("clear-history", false, "Clears the history")
	flagSystemTheme       = flag.Bool("system-theme", false, "Uses the system gtk theme")
	flagMaxEntries        = flag.Int("max-entries", 100, "Maximum amount of clipboard entries the history should hold")
	flagLayerShell        = flag.Bool("layer-shell", true, "Use layer shell instead of window")
	flagSilent            = flag.Bool("silent", false, "Starts bbclip silently in the background")
	flagIcons             = flag.Bool("icons", false, "")
	flagTextPreviewLength = flag.Int("text-preview-length", 100, "The length of the preview text before it's truncated")
	flagImageSupport      = flag.Bool("image-support", false, "Whether to enable image support")
	flagImageHeight       = flag.Int("image-height", 50, "Image height")
	flagImagePreview      = flag.Bool("image-preview", true, "Whether to show a tiny preview of the image")
	flagPreviewWidth      = flag.Int("preview-width", 300, "The width of the preview window")
	flagShowPreview       = flag.Bool("show-preview", false, "Whether to show the preview window by default when opening bbclip.")
)

type BBClip struct {
	// history is the clipboard history
	history *History

	// window is the gtk window
	window *gtk.Window

	// the windowWrapper which contains the item list and the preview
	windowWrapper *gtk.Box

	// previewFrame is the preview window
	previewFrame       *gtk.Frame
	previewScrolledWin *gtk.ScrolledWindow

	// previewBox is the wrapper of the preview window containing
	// the text view and the image box
	previewBox *gtk.Box

	previewTextView   *gtk.TextView
	previewTextBuffer *gtk.TextBuffer

	// previewImgBox is the GtkBox containing the preview image
	previewImgBox *gtk.Box
	previewImg    *gtk.Image

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

	entryItemsContent map[int]HistoryEntry

	// cssProvider is the gtk css provider
	cssProvider *gtk.CssProvider

	visTime time.Time

	conf *Config
}

func main() {
	flag.Parse()

	if *flagShowVersion {
		printVersion()
		return
	}

	if tryConnectSocket() {
		fmt.Println("Another instance already running. Exiting.")
		return
	}

	gtk.Init(nil)

	bbclip := BBClip{
		history: &History{},
		conf:    NewConfig(),
	}

	bbclip.buildUi()
	bbclip.listenSocket()

	if !bbclip.conf.BoolVal(Silent, *flagSilent) {
		bbclip.window.ShowAll()
		bbclip.window.Present()

		if !bbclip.conf.BoolVal(ShowPreview, *flagShowPreview) {
			// since the preview window is built before the main window is shown
			// ShowAll would also display the preview window by default.
			// So we close it initially
			bbclip.togglePreview()
		}

		glib.IdleAdd(func() {
			if bbclip.window.IsVisible() {
				bbclip.goToTop()
				bbclip.refreshEntryList(
					initialItems+1,
					bbclip.history.maxEntries,
				)
			}
		})
	}

	gtk.Main()
}

// buildUi builds the main ui, applies css style and populates the
// clipboard history.
func (b *BBClip) buildUi() {
	b.history = NewHistory(b.conf)
	b.history.Init()

	var err error

	b.window, err = gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		log.Fatal("Unable to create window:", err)
	}

	b.windowWrapper, _ = gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 8)

	if b.conf.BoolVal(LayerShell, *flagLayerShell) {
		b.buildLayerShell()
	}

	b.buildSearchBar()
	b.buildEntriesList()
	b.refreshEntryList(0, initialItems)
	b.buildWindow()

	b.popupWrapper, _ = gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 8)
	b.popupWrapper.PackStart(b.search, true, true, 0)
	b.popupWrapper.PackStart(b.entriesListWrapper, true, true, 0)
	b.windowWrapper.PackStart(b.popupWrapper, true, true, 8)

	b.buildPreview()
	b.applyStyles()

	b.window.Add(b.windowWrapper)
}

func (b *BBClip) buildWindow() {
	b.window.SetTitle("bellbird clipboard")
	b.window.SetDecorated(false)
	b.window.SetDefaultSize(defaultWidth, defaultHeight)
	b.window.SetResizable(false)
	b.window.SetAppPaintable(true)
	b.window.Connect("key-press-event", b.onKeyPress)
	b.window.Connect("focus-out-event", b.onFocusOut)
}

func (b *BBClip) buildLayerShell() {
	layershell.InitForWindow(b.window)
	layershell.SetNamespace(b.window, "gtk-layer-shell")
	layershell.SetAnchor(b.window, layershell.LAYER_SHELL_EDGE_TOP, false)
	layershell.SetMargin(b.window, layershell.LAYER_SHELL_EDGE_TOP, 0)
	layershell.SetMargin(b.window, layershell.LAYER_SHELL_EDGE_LEFT, 0)
	layershell.SetMargin(b.window, layershell.LAYER_SHELL_EDGE_RIGHT, 0)
	layershell.SetExclusiveZone(b.window, 30)
	layershell.SetKeyboardMode(b.window, layershell.LAYER_SHELL_KEYBOARD_MODE_EXCLUSIVE)
	layershell.SetLayer(b.window, layershell.LAYER_SHELL_LAYER_OVERLAY)
}

func (b *BBClip) buildSearchBar() {
	b.search, _ = gtk.EntryNew()
	b.search.SetCanFocus(false)
	b.search.SetIconFromIconName(gtk.ENTRY_ICON_PRIMARY, "system-search")
	b.search.SetPlaceholderText("Search...")
	b.search.Connect("button-press-event", b.onButtonPress)
	b.search.Connect("key-release-event", b.onKeyRelease)
}

func (b *BBClip) buildEntriesList() {
	b.entriesList, _ = gtk.ListBoxNew()
	b.entriesList.SetSelectionMode(gtk.SELECTION_SINGLE)
	b.entriesList.SetMarginBottom(6)
	b.entriesList.Connect("row-activated", b.onRowActivated)
	b.entryItemsContent = make(map[int]HistoryEntry)

	b.entriesListWrapper, _ = gtk.ScrolledWindowNew(nil, nil)
	b.entriesListWrapper.SetSizeRequest(defaultWidth, defaultHeight)
	b.entriesListWrapper.SetOverlayScrolling(true)
	b.entriesListWrapper.Add(b.entriesList)
}

// buildPreview build the preview window containing a scrollable window,
// a textview, a text buffer and an image
func (b *BBClip) buildPreview() {
	tagTable, _ := gtk.TextTagTableNew()
	b.previewTextBuffer, _ = gtk.TextBufferNew(tagTable)
	b.previewTextView, _ = gtk.TextViewNewWithBuffer(b.previewTextBuffer)
	b.previewTextView.SetEditable(false)
	b.previewTextView.SetCanFocus(false)
	b.previewTextView.SetWrapMode(gtk.WRAP_NONE)

	b.previewFrame, _ = gtk.FrameNew("")
	b.previewFrame.SetShadowType(gtk.SHADOW_NONE)
	b.previewFrame.SetAppPaintable(true)
	b.previewFrame.Add(b.previewTextView)

	width := b.conf.IntVal(PreviewWidth, *flagPreviewWidth)

	b.previewScrolledWin, _ = gtk.ScrolledWindowNew(nil, nil)
	b.previewScrolledWin.Add(b.previewFrame)
	b.previewScrolledWin.SetSizeRequest(width, defaultHeight)

	b.previewImg, _ = gtk.ImageNew()
	b.previewImgBox, _ = gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	b.previewImgBox.PackStart(&b.previewImg.Widget, true, true, 0)

	b.previewBox, _ = gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	b.previewBox.SetSizeRequest(width, defaultHeight)
	b.previewBox.PackStart(b.previewScrolledWin, true, true, 0)
	b.previewBox.PackEnd(b.previewImgBox, true, true, 0)

	b.windowWrapper.PackEnd(b.previewBox, true, true, 0)
}

// togglePreview displays or hides the preview window depending on the
// current visibility
func (b *BBClip) togglePreview() {
	if b.search.HasFocus() {
		return
	}

	width := b.conf.IntVal(PreviewWidth, *flagPreviewWidth)

	if b.previewBox.IsVisible() {
		b.previewBox.Hide()
		b.window.Resize(400, 400)
	} else {
		b.previewBox.ShowAll()
		b.updatePreviewContent()
		b.window.Resize(400+width, 400)
	}
}

func (b *BBClip) updatePreviewContent() error {
	row := b.entriesList.GetSelectedRow()
	entry := b.entryItemsContent[row.GetIndex()]

	b.previewScrolledWin.Hide()
	b.previewImgBox.Hide()

	if entry.img != nil {
		b.previewImgBox.ShowAll()

		parsedUrl, err := url.Parse(*entry.str)
		if err != nil {
			return err
		}

		imgPath := parsedUrl.Path
		pixbuf, err := gdk.PixbufNewFromFile(imgPath)
		if err != nil {
			return err
		}

		var (
			width      = b.conf.IntVal(PreviewWidth, *flagPreviewWidth)
			height     = defaultHeight
			origWidth  = pixbuf.GetWidth()
			origHeight = pixbuf.GetHeight()
		)

		// Calculate aspect-preserving scale
		scale := 1.0
		if origWidth > width || origHeight > height {
			scaleWidth := float64(width) / float64(origWidth)
			scaleHeight := float64(height) / float64(origHeight)
			scale = math.Min(scaleWidth, scaleHeight)
		}

		newWidth := int(float64(origWidth) * scale)
		newHeight := int(float64(origHeight) * scale)

		scaledPixBuf, _ := pixbuf.ScaleSimple(
			newWidth,
			newHeight,
			gdk.INTERP_BILINEAR,
		)

		if b.previewImg != nil {
			b.previewImgBox.Remove(&b.previewImg.Widget)
		}

		img, _ := gtk.ImageNewFromPixbuf(scaledPixBuf)
		img.SetHAlign(gtk.ALIGN_CENTER)
		img.Show()

		b.previewImgBox.PackStart(&img.Widget, false, false, 0)
		b.previewImg = img
	} else {
		if entry.str == nil {
			return errors.New("no entry found")
		}

		b.previewScrolledWin.ShowAll()
		b.previewTextBuffer.SetText(*entry.str)
	}

	return nil
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
					b.refreshEntryList(0, initialItems)
					b.window.ShowAll()
					b.window.Present()

					if !b.conf.BoolVal(ShowPreview, *flagShowPreview) {
						// since the preview window is built before the main
						// window is shown ShowAll would also display the p
						// review window by default. So we close it initially
						b.togglePreview()
					}

					b.goToTop()
					glib.IdleAdd(func() {
						if b.window.IsVisible() {
							b.refreshEntryList(
								initialItems+1,
								b.history.maxEntries,
							)
						}
					})
					b.visTime = time.Now()
				})
			}
			conn.Close()
		}
	}()
}

func (b *BBClip) handleKeyEvents(key *gdk.EventKey) bool {
	name := gdk.KeyValName(key.KeyVal())
	sinceShow := time.Since(b.visTime)

	switch name {
	case "Escape":
		if b.search.HasFocus() {
			b.focusEntryList()
			return true
		} else {
			// On some occasions the escape key gets magically triggered
			// for somereaonse right after the window was called.
			// Delaying the action seems to fix it.
			if sinceShow > 200*time.Millisecond {
				b.window.Hide()
			}
		}

	case "Delete", "D":
		b.deleteSelectedRow()
	case "Return":
		if b.entriesList != nil && sinceShow > 200*time.Millisecond {
			row := b.entriesList.GetSelectedRow()
			b.selectAndHide(row)
		}

	case "k", "Up":
		b.rowUp()

	case "j", "Down":
		b.rowDown()

	case "g":
		b.goToTop()

	case "G":
		b.goToBottom()

	case "p":
		b.togglePreview()

	case "i", "slash":
		if !b.search.HasFocus() {
			b.search.SetCanFocus(true)
			b.search.GrabFocus()
			return true
		}
	}

	if key.State()&gdk.CONTROL_MASK != 0 {
		switch key.KeyVal() {
		case gdk.KEY_u:
			b.halfViewUp()

		case gdk.KEY_d:
			b.halfViewDown()

		case gdk.KEY_c:
			gtk.MainQuit()
		}
	}

	if b.previewBox.IsVisible() {
		b.updatePreviewContent()
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

		rowContent := *b.entryItemsContent[row.GetIndex()].str

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

	entry := b.entryItemsContent[row.GetIndex()]

	if *entry.str == "" {
		b.window.Hide()
		return
	}

	if ok, index := b.history.contains(*entry.str); ok {
		entries := b.history.entries
		b.history.entries = slices.Delete(entries, index, index+1)
		b.history.entries = append(b.history.entries, entry)
	}

	if err := b.history.WriteToClipboard(entry); err != nil {
		println("Could not write to clipboard:", err)
	}

	b.window.Hide()
}

// deleteSelectedRow removes the selected row from the clipboard history
// and moves the selected row to the same spot of the previously
// selected row.
func (b *BBClip) deleteSelectedRow() {
	if b.entriesList == nil {
		return
	}

	row := b.entriesList.GetSelectedRow()
	if row == nil {
		return
	}

	rowIndex := row.GetIndex()
	rowName, _ := row.GetName()
	entryIndex, _ := strconv.Atoi(rowName)

	if index, _ := b.history.removeEntry(entryIndex); index > -1 {
		b.refreshEntryList(0, b.history.maxEntries)

		if len(b.history.entries) == 0 {
			return
		}

		// move the selection to the same spot the deletion took place
		rowIndex = Clamp(rowIndex, 0, len(b.history.entries)-1)
		prevRow := b.entriesList.GetRowAtIndex(rowIndex)

		if prevRow.IsVisible() {
			b.entriesList.SelectRow(prevRow)
		}
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

// halfViewUp moves the selection one row up and repositions the view if needed
func (b *BBClip) halfViewUp() {
	_, _, rowsInView := b.rowInfo()

	for range rowsInView / 2 {
		b.rowUp()
	}

	b.repositionView()
}

// halfViewDown moves the selection one row up and repositions the view if needed
func (b *BBClip) halfViewDown() {
	_, _, rowsInView := b.rowInfo()

	for range rowsInView / 2 {
		b.rowDown()
	}

	b.repositionView()
}

func (b *BBClip) rowInfo() (rowHeight int, listViewHeight int, rowsInView int) {
	rowHeight = b.entriesList.GetSelectedRow().GetAllocatedHeight()
	listViewHeight = b.entriesListWrapper.GetAllocatedHeight()
	rowsInView = listViewHeight / rowHeight

	return rowHeight, listViewHeight, rowsInView
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

func (b *BBClip) goToBottom() {
	rowCount := int(b.entriesList.GetChildren().Length())
	index := b.entriesList.GetRowAtIndex(rowCount - 1)
	b.entriesList.SelectRow(index)
	b.repositionView()
}

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
// rebuilds the clipboard history list and automatically sets focus
// to the history list
func (b *BBClip) refreshEntryList(from int, to int) {
	b.history.mu.RLock()
	defer b.history.mu.RUnlock()

	if b.entriesList == nil {
		return
	}

	// if from is greater than zero we're most likely want to append
	// to the existing list rather than rebuilding
	if from == 0 {
		children := b.entriesList.GetChildren()
		for e := children; e != nil; e = e.Next() {
			child := e.Data().(*gtk.Widget)
			b.entriesList.Remove(child)
			child.Destroy()
		}
	}

	// get a copy of the entries in reversed order so that we can
	// display the last added history entry as the first item
	entries := Reverse(b.history.entries)

	for i := range entries {
		// skip certain entries
		if i < from || i > to {
			continue
		}

		preview := strings.ReplaceAll(*entries[i].str, "\n", "↲")
		// truncate the preview
		textLength := b.conf.IntVal(TextPreviewLen, *flagTextPreviewLength)
		preview = TruncateText(preview, textLength)

		// the real clipboard entry
		entry := entries[i]
		isImg := entry.img != nil

		rowBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)

		iconName := "text-x-generic-symbolic"
		if isImg {
			iconName = "image-x-generic-symbolic"
		}

		imageSupport := b.conf.BoolVal(ImageSupport, *flagImageSupport)
		if imageSupport && isImg && b.conf.BoolVal(ImagePreview, *flagImagePreview) {
			// get local filepath
			parsedUrl, err := url.Parse(*entry.str)
			if err != nil {
				continue
			}

			imgPath := parsedUrl.Path

			// if there's an Image object that get that path
			if entry.img.path != "" {
				imgPath = entry.img.path
			}

			img, err := b.createEntryImage(
				imgPath,
				b.conf.IntVal(ImageHeight, *flagImageHeight),
			)
			if err == nil {
				rowBox.PackEnd(img, true, true, 8)
			}
		} else {
			label, _ := gtk.LabelNew(preview)
			label.SetLineWrap(true)
			label.SetLineWrapMode(pango.WRAP_WORD_CHAR)
			label.SetMarginTop(6)
			label.SetMarginBottom(6)
			label.SetXAlign(0)
			rowBox.PackEnd(label, true, true, 8)
			b.addContextClass(label.ToWidget(), "entries-list-row-label")
		}

		if b.conf.BoolVal(Icons, *flagIcons) {
			icon, _ := gtk.ImageNewFromIconName(iconName, gtk.ICON_SIZE_BUTTON)
			if !isImg {
				icon.SetVAlign(gtk.ALIGN_START)
				icon.SetMarginTop(6)
			}
			rowBox.PackStart(icon, false, false, 0)
			b.addContextClass(icon.ToWidget(), "entries-list-row-icon")
		}

		row, _ := gtk.ListBoxRowNew()
		row.SetName(strconv.Itoa(i))
		row.Add(rowBox)
		row.ShowAll()

		b.addContextClass(row.ToWidget(), "entries-list-row")

		b.entriesList.Add(row)
		b.entryItemsContent[row.GetIndex()] = entry
	}

	b.entriesList.GrabFocus()
	b.search.SetCanFocus(false)
	b.search.SetText("")
}

func (b *BBClip) createEntryImage(imgPath string, height int) (*gtk.Image, error) {
	pixbuf, err := gdk.PixbufNewFromFile(imgPath)
	if err != nil {
		return nil, err
	}

	w := pixbuf.GetWidth()
	h := pixbuf.GetHeight()

	scale := float64(height) / float64(h)
	w = int(float64(w) * scale)
	h = height

	scaledPixBuf, _ := pixbuf.ScaleSimple(w, h, gdk.INTERP_BILINEAR)

	img, _ := gtk.ImageNewFromPixbuf(scaledPixBuf)
	img.SetHAlign(gtk.ALIGN_START)

	return img, nil
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

	if err := b.injectUserStyles(screen); err != nil {
		fmt.Println(err)
	}

	if !b.conf.BoolVal(SystemTheme, *flagSystemTheme) {
		b.addContextClass(&b.window.Widget, "bbclip")
	}

	b.addContextClass(&b.search.Widget, "search")
	b.addContextClass(&b.entriesListWrapper.Widget, "entries-list-wrapper")
	b.addContextClass(&b.entriesList.Widget, "entries-list")
	b.addContextClass(&b.windowWrapper.Widget, "popup-wrapper")
	b.addContextClass(&b.previewBox.Widget, "preview-wrapper")
	b.addContextClass(&b.previewTextView.Widget, "preview")
}

func (b *BBClip) injectUserStyles(screen *gdk.Screen) error {
	confDir, _ := ConfigDir()
	userFile := confDir + "/" + userCssFileName

	if _, err := os.Stat(userFile); err == nil {
		userProvider, _ := gtk.CssProviderNew()

		if err := userProvider.LoadFromPath(userFile); err != nil {
			return fmt.Errorf("Unabled to load or parse user CSS: %s", err)
		}

		gtk.AddProviderForScreen(
			screen,
			userProvider,
			gtk.STYLE_PROVIDER_PRIORITY_APPLICATION,
		)
	}

	return nil
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
	if time.Since(b.visTime) > 200*time.Millisecond {
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
	runes := []rune(text)
	if len(runes) > maxWidth {
		if maxWidth > 3 {
			return string(runes[:maxWidth-3]) + "..."
		}
		return string(runes[:maxWidth]) // No space for "..."
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

func IsFlagPassed(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}
