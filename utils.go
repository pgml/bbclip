package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

func clipboardHasImage() (Image, bool) {
	cmd := exec.Command("wl-paste", "--list-types")
	out, err := cmd.Output()

	image := Image{}
	if err != nil {
		return image, false
	}

	for line := range bytes.SplitSeq(out, []byte("\n")) {
		var imageSrc ImageSource
		mimeType := ""

		if strings.HasPrefix(string(line), "-moz-url") {
			imageSrc = ImageSrcBrowser
			mimeType = "image/*"
		} else if strings.HasPrefix(string(line), "text/uri-list") {
			imageSrc = ImageSrcFileSystem
			mimeType = "image/*"
		}

		if strings.HasPrefix(string(line), "image/") {
			mimeType = string(line)
		}

		if mimeType != "" {
			return Image{
				source:   imageSrc,
				mimeType: mimeType,
			}, true
		}
	}
	return image, false
}

func downloadImage(imgUrl string, img *Image) error {
	req, err := http.NewRequest("GET", imgUrl, nil)
	if err != nil {
		log.Fatal(err)
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	client := &http.Client{}
	r, err := client.Do(req)

	r, err = http.Get(imgUrl)
	if err != nil {
		log.Fatal(err)
		return err
	}
	defer r.Body.Close()

	savePath := urlToCachePath(imgUrl)

	if _, err := os.Stat(savePath); err != nil {
		f, err := os.Create(savePath)
		if err != nil {
			log.Fatal(err)
			return err
		}
		defer f.Close()

		n, err := io.Copy(f, r.Body)

		if err != nil {
			log.Fatal(err)
			return err
		}

		img.path = savePath
		img.size = n
	}

	return nil
}

func extractPathFromImgTag(imgTag string) string {
	re := regexp.MustCompile(`(?i)<img[^>]+src=["']?([^"' >]+)["']?`)
	matches := re.FindStringSubmatch(imgTag)

	if len(matches) > 0 {
		return matches[1]
	}
	return ""
}

func fileUrl(path string, img *Image) string {
	if strings.HasPrefix(path, "file://") {
		return path
	}

	if img != nil {
		switch img.source {
		case ImageSrcBrowser:
			path = extractPathFromImgTag(path)
			img.path = path
			path = urlToCachePath(path)
		}
	}

	url := url.URL{
		Scheme: "file",
		Path:   filepath.ToSlash(path),
	}

	return url.String()
}

func urlToCachePath(imgUrl string) string {
	cacheDir, _ := CacheDir()
	imgUrl = strings.SplitAfter(imgUrl, "?")[0]
	imgUrl = strings.TrimSuffix(imgUrl, "?")

	fname := path.Base(imgUrl)
	return cacheDir + "/" + fname
}
