package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"slices"
	"sync"
	"time"

	"github.com/adrg/xdg"
)

const HistoryFile = "org.pgml.bbclip-hist"

type History struct {
	mu         sync.RWMutex
	maxEntries int
	entries    []string
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
				last = h.entries[len(h.entries)-1]
			}

			if cont != last {
				h.mu.Lock()

				if ok, index := h.contains(cont); ok {
					h.entries = slices.Delete(h.entries, index, index+1)
				}

				h.entries = append(h.entries, cont)

				if err := h.Save(); err != nil {
					println("Could not save to clipboard history:", err)
				}

				h.mu.Unlock()
			}
		}
	}()
}

func (h *History) Read() ([]string, error) {
	path := xdg.DataHome + "/" + HistoryFile
	file, err := os.OpenFile(path, os.O_RDONLY, 0644)

	if err != nil {
		return []string{}, err
	}
	defer file.Close()

	var history []string

	err = json.NewDecoder(file).Decode(&history)

	return history, nil

}

func (h *History) Save() error {
	file, err := os.OpenFile(h.path, os.O_WRONLY|os.O_TRUNC, 0644)

	if err != nil {
		println(err)
		return err
	}
	defer file.Close()

	return json.NewEncoder(file).Encode(h.entries)
}

func (h *History) WriteToClipboard(text string) error {
	cmd := exec.Command("wl-copy", "--type", "text/plain", "--foreground")
	stdin, err := cmd.StdinPipe()

	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	go func() {
		defer stdin.Close()
		io.WriteString(stdin, text)
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
	if slices.Contains(h.entries, content) {
		return true, slices.Index(h.entries, content)
	}

	return false, -1
}
