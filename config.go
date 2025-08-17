package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	confDirName  = "bbclip"
	userConfFile = "config"
)

type ConfigOption int

const (
	SystemTheme ConfigOption = iota
	MaxEntries
	LayerShell
	Silent
	Icons
	TextPreviewLen
	ImageSupport
	ImageHeight
	ImagePreview
)

type Option struct {
	key string
	arg any
}

var options = map[ConfigOption]Option{
	SystemTheme:  {"system-theme", flagSystemTheme},
	MaxEntries:   {"max-entries", *flagMaxEntries},
	LayerShell:   {"layer-shell", *flagLayerShell},
	Silent:       {"silent", *flagSilent},
	Icons:        {"icons", *flagIcons},
	ImageSupport: {"image-support", *flagImageSupport},
	ImageHeight:  {"image-height", *flagImageHeight},
	ImagePreview: {"image-preview", *flagImagePreview},
}

func (o ConfigOption) String() string {
	return options[o].key
}
func (o ConfigOption) Arg() any {
	return options[o].arg
}

type Config struct {
	file   string
	values map[string]string
}

func NewConfig() *Config {
	confDir, _ := ConfigDir()
	configFile := confDir + "/" + userConfFile

	conf := Config{
		file:   configFile,
		values: make(map[string]string, 0),
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

		c.values[key] = val
	}

	return nil
}

func (c *Config) BoolVal(opt ConfigOption, defaultVal bool) bool {
	val, ok := c.values[opt.String()]

	if ok && !IsFlagPassed(options[opt].key) {
		return val == "true"
	}

	return defaultVal
}

func (c *Config) IntVal(opt ConfigOption, defaultVal int) int {
	val, ok := c.values[opt.String()]

	if ok && !IsFlagPassed(options[opt].key) {
		v, _ := strconv.Atoi(val)
		return v
	}

	return defaultVal
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

// ConfigDir returns the config directory
func CacheDir() (string, error) {
	ConfigDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}

	confDir := filepath.Join(ConfigDir, confDirName)

	if _, err := os.Stat(confDir); err != nil {
		os.Mkdir(confDir, 0755)
	}

	return confDir, nil
}
