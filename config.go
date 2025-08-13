package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

//var conf = struct {
//	SystemTheme, MaxEntries, LayerShell string
//}{
//	SystemTheme: "system-theme",
//	MaxEntries:  "max-entries",
//	LayerShell:  "layer-shell",
//}

type ConfOption int

const (
	SystemTheme ConfOption = iota
	MaxEntries
	LayerShell
)

var options = map[ConfOption]string{
	SystemTheme: "system-theme",
	MaxEntries:  "max-entries",
	LayerShell:  "layer-shell",
}

func (o ConfOption) String() string {
	return options[o]
}

type Config struct {
	file        string
	maxEntries  int
	systemTheme bool
	layerShell  bool
}

func NewConfig() *Config {
	confDir, _ := ConfigDir()
	configFile := confDir + "/" + userConfFile

	conf := Config{
		file:        configFile,
		maxEntries:  100,
		layerShell:  true,
		systemTheme: false,
	}
	conf.read()

	return &conf
}

func (c *Config) read() error {
	file, err := os.OpenFile(c.file, os.O_RDONLY, 0644)

	if err != nil {
		return err
	}

	defer file.Close()

	reader := bufio.NewReader(file)

	for {
		line, err := reader.ReadSlice('\n')
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
		}

		ln := strings.Split(string(line), "=")
		key := strings.TrimSpace(ln[0])
		val := strings.TrimSpace(ln[1])

		// @todo make this better
		switch key {
		case "max-entries":
			c.maxEntries, _ = strconv.Atoi(val)

		case "layer-shell":
			c.layerShell = val == "true"

		case "system-theme":
			c.systemTheme = val == "true"
		}
	}

	return nil
}

// ConfigDir returns the config directory
func ConfigDir() (string, error) {
	ConfigDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	confDir := filepath.Join(ConfigDir, confDirName)

	if _, err := os.Stat(confDir); err != nil {
		os.Mkdir(confDir, 0755)
	}

	return confDir, nil
}
