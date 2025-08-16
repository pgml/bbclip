package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"sync"
	"time"

	"github.com/adrg/xdg"
)

const HistoryFile = "org.pgml.bbclip-hist"

type ImageSource int

const (
	ImageSrcBrowser ImageSource = iota
	ImageSrcFileSystem
)

type Image struct {
	source   ImageSource
	mimeType string
	path     string
	size     int64
}

type HistoryEntry struct {
	str *string
	img *Image
}

type History struct {
	mu         sync.RWMutex
	maxEntries int
	entries    []HistoryEntry
	path       string
}

func NewHistory(maxEntries int) *History {
	path := xdg.DataHome + "/" + HistoryFile

	if _, err := os.Stat(path); err != nil {
		flags := os.O_CREATE | os.O_RDONLY

		f, err := os.OpenFile(path, flags, 0644)
		if err != nil {
			println(err)
		}
		defer f.Close()
	}

	history := &History{
		mu: sync.RWMutex{},
		// @todo make config option `max-entries = 100`
		maxEntries: maxEntries,
		path:       path,
	}

	if *flagClearHistory {
		history.clear()
	}

	history.mu.RLock()
	entries, err := history.Read()
	history.entries = entries
	history.mu.RUnlock()

	if err != nil {
		println(err)
	}

	if len(history.entries) > history.maxEntries {
		exceeds := len(history.entries) - history.maxEntries
		history.entries = slices.Delete(history.entries, 0, 0+exceeds)
		history.Save()
	}

	history.cleanCache()

	return history
}

func (h *History) Init() {
	go func() {
		ticker := time.NewTicker(300 * time.Millisecond)
		defer ticker.Stop()

		for range ticker.C {
			out, err := exec.Command("wl-paste", "--no-newline").Output()
			if err != nil {
				continue
			}

			cont := string(bytes.TrimSpace(out))
			if cont == "" {
				continue
			}

			last := ""
			if len(h.entries) > 0 {
				last = *h.entries[len(h.entries)-1].str
			}

			shouldRefresh := false
			historyEntry := HistoryEntry{}

			if img, ok := clipboardHasImage(); ok {
				fileUrl := fileUrl(string(cont), &img)

				if fileUrl != last {
					if img.source == ImageSrcBrowser {
						p, _ := downloadImage(img.path)
						img.path = p
					}
					historyEntry.str = &fileUrl
					historyEntry.img = &img
					shouldRefresh = true
				}
			} else {
				if cont != last {
					historyEntry.str = &cont
					shouldRefresh = true
				}
			}

			if shouldRefresh {
				h.mu.Lock()

				if ok, index := h.contains(cont); ok {
					h.entries = slices.Delete(h.entries, index, index+1)
				}

				h.entries = append(h.entries, historyEntry)

				if err := h.Save(); err != nil {
					println("Could not save to clipboard history:", err)
				}

				h.mu.Unlock()
			}
		}
	}()
}

func (h *History) Read() ([]HistoryEntry, error) {
	path := xdg.DataHome + "/" + HistoryFile
	file, err := os.OpenFile(path, os.O_RDONLY, 0644)

	if err != nil {
		return []HistoryEntry{}, err
	}
	defer file.Close()

	var history []string

	err = json.NewDecoder(file).Decode(&history)

	entries := []HistoryEntry{}
	for _, entry := range history {
		var img *Image = nil
		if fileUrl, fErr := url.Parse(entry); fErr == nil {
			if fileUrl.Scheme == "file" {
				if f, err := os.Stat(fileUrl.Path); err == nil {
					img = &Image{
						source:   ImageSrcFileSystem,
						mimeType: "image/*",
						path:     fileUrl.Path,
						size:     f.Size(),
					}
				}
			}
		}

		//fmt.Println(entry, img)
		entries = append(entries, HistoryEntry{
			str: &entry,
			img: img,
		})
	}

	return entries, nil

}

func (h *History) Save() error {
	file, err := os.OpenFile(h.path, os.O_WRONLY|os.O_TRUNC, 0644)

	if err != nil {
		println(err)
		return err
	}
	defer file.Close()

	entries := []string{}
	for _, entry := range h.entries {
		if entry.str == nil {
			continue
		}
		entries = append(entries, *entry.str)
	}

	return json.NewEncoder(file).Encode(entries)
}

func (h *History) WriteToClipboard(entry HistoryEntry) error {
	mimeType := "text/plain"
	cpContent := *entry.str

	if entry.img != nil {
		mimeType = "text/uri-list"
		cpContent = *entry.str
	}

	cmd := exec.Command("wl-copy", "--type", mimeType, "--foreground")
	stdin, err := cmd.StdinPipe()

	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	go func() {
		defer stdin.Close()
		io.WriteString(stdin, cpContent)
	}()

	go func() {
		cmd.Wait()
	}()

	return nil
}

func (h *History) removeEntry(index int) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if index >= len(h.entries) {
		return -1, errors.New("No entry found")
	}

	// check if we're deleting the last entry (which would be the first
	// entry in the history view in the gui)
	isLastEntry := index == len(h.entries)-1

	h.entries = slices.Delete(h.entries, index, index+1)

	// is last entry, we set the clipboard to the new last entry
	// so that the goroutine doesn't override our deletion
	if isLastEntry {
		h.WriteToClipboard(h.entries[len(h.entries)-1])
	}

	if err := h.Save(); err != nil {
		println("Could not save to clipboard history:", err)
		return -1, err
	}
	return index, nil
}

func (h *History) clear() error {
	f, err := os.OpenFile(h.path, os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString("")

	if err != nil {
		return err
	}

	return nil
}

func (h *History) contains(content string) (bool, int) {
	for i, entry := range h.entries {
		if *entry.str == content {
			return true, i
		}
	}

	return false, -1
}

func (h *History) cleanCache() error {
	cacheDir, _ := CacheDir()

	if dir, err := os.ReadDir(cacheDir); err == nil {
		for _, d := range dir {
			path := cacheDir + "/" + d.Name()
			f := fileUrl(path, nil)

			if ok, _ := h.contains(f); !ok {
				if err := os.Remove(path); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
