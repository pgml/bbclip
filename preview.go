package main

import (
	"errors"
	"math"
	"net/url"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

type Preview struct {
	// previewFrame is the preview window
	frame       *gtk.Frame
	scrolledWin *gtk.ScrolledWindow
	// previewBox is the wrapper of the preview window containing
	// the text view and the image box
	box        *gtk.Box
	textView   *gtk.TextView
	textBuffer *gtk.TextBuffer
	// previewImgBox is the GtkBox containing the preview image
	imgBox      *gtk.Box
	img         *gtk.Image
	conf        *Config
	entriesList *EntriesList
	window      *gtk.Window
}

// NewPreview builds the preview window containing a scrollable window,
// a textview, a text buffer and an image
func NewPreview(
	conf *Config,
	entriesList *EntriesList,
	win *gtk.Window,
) *Preview {
	tagTable, _ := gtk.TextTagTableNew()
	p := Preview{}
	p.textBuffer, _ = gtk.TextBufferNew(tagTable)
	p.textView, _ = gtk.TextViewNewWithBuffer(p.textBuffer)
	p.textView.SetEditable(false)
	p.textView.SetCanFocus(false)
	p.textView.SetWrapMode(gtk.WRAP_NONE)

	p.frame, _ = gtk.FrameNew("")
	p.frame.SetShadowType(gtk.SHADOW_NONE)
	p.frame.SetAppPaintable(true)
	p.frame.Add(p.textView)

	width := conf.IntVal(PreviewWidth, *flagPreviewWidth)

	p.scrolledWin, _ = gtk.ScrolledWindowNew(nil, nil)
	p.scrolledWin.Add(p.frame)
	p.scrolledWin.SetSizeRequest(width, defaultHeight)

	p.img, _ = gtk.ImageNew()
	p.imgBox, _ = gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	p.imgBox.PackStart(&p.img.Widget, true, true, 0)

	p.box, _ = gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	p.box.SetSizeRequest(width, defaultHeight)
	p.box.PackStart(p.scrolledWin, true, true, 0)
	p.box.PackEnd(p.imgBox, true, true, 0)

	p.conf = conf
	p.entriesList = entriesList
	p.window = win

	return &p
}

// toggle displays or hides the preview window depending on the
// current visibility
func (p *Preview) toggle() {
	width := p.conf.IntVal(PreviewWidth, *flagPreviewWidth)

	if p.box.IsVisible() {
		p.box.Hide()
		p.window.Resize(400, 400)
	} else {
		p.box.ShowAll()
		p.update()
		p.window.Resize(400+width, 400)
	}
}

func (p *Preview) update() error {
	row := p.entriesList.box.GetSelectedRow()
	entry := p.entriesList.items[row.GetIndex()]

	p.scrolledWin.Hide()
	p.imgBox.Hide()

	if entry.img != nil {
		p.imgBox.ShowAll()

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
			width      = p.conf.IntVal(PreviewWidth, *flagPreviewWidth)
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

		if p.img != nil {
			p.imgBox.Remove(&p.img.Widget)
		}

		img, _ := gtk.ImageNewFromPixbuf(scaledPixBuf)
		img.SetHAlign(gtk.ALIGN_CENTER)
		img.Show()

		p.imgBox.PackStart(&img.Widget, false, false, 0)
		p.img = img
	} else {
		if entry.str == nil {
			return errors.New("no entry found")
		}

		p.scrolledWin.ShowAll()
		p.textBuffer.SetText(*entry.str)
	}

	return nil
}
