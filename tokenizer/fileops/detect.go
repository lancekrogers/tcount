package fileops

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Common binary file extensions.
var binaryExtensions = map[string]bool{
	// Images
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".bmp": true, ".ico": true, ".webp": true,
	".tiff": true, ".tif": true,

	// Documents
	".pdf": true, ".doc": true, ".docx": true,
	".xls": true, ".xlsx": true, ".ppt": true, ".pptx": true,

	// Archives
	".zip": true, ".tar": true, ".gz": true,
	".bz2": true, ".7z": true, ".rar": true,

	// Executables
	".exe": true, ".dll": true, ".so": true,
	".dylib": true, ".app": true, ".bin": true,

	// Audio/Video
	".mp3": true, ".mp4": true, ".avi": true, ".mov": true,
	".wmv": true, ".flv": true, ".wav": true, ".flac": true, ".ogg": true,

	// Fonts
	".ttf": true, ".otf": true, ".woff": true, ".woff2": true,

	// Other
	".pyc": true, ".class": true, ".o": true, ".a": true,
	".tiktoken": true,
}

// binarySniffBytes is the prefix window scanned for null bytes, matching the
// convention used by git and http.DetectContentType.
const binarySniffBytes = 512

// IsBinaryFile checks if a file is likely binary.
func IsBinaryFile(path string, collectors ...BinaryStatsCollector) (bool, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if binaryExtensions[ext] {
		return true, nil
	}

	var collector BinaryStatsCollector
	if len(collectors) > 0 {
		collector = collectors[0]
	}
	var readStarted time.Time
	if collector != nil {
		readStarted = time.Now()
		defer func() {
			collector.RecordValidationReadDuration(time.Since(readStarted))
		}()
	}

	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	if collector != nil {
		collector.RecordBinarySniffOpen()
	}
	defer func() { _ = file.Close() }()

	buf := make([]byte, binarySniffBytes)
	n, err := file.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		if collector != nil {
			collector.RecordBinarySniffBytes(int64(n))
		}
		return false, err
	}
	if collector != nil {
		collector.RecordBinarySniffBytes(int64(n))
	}

	if bytes.Contains(buf[:n], []byte{0}) {
		return true, nil
	}

	return false, nil
}
