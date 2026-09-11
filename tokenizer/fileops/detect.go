package fileops

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
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
	".wasm": true,

	// Audio/Video
	".mp3": true, ".mp4": true, ".avi": true, ".mov": true,
	".wmv": true, ".flv": true, ".wav": true, ".flac": true, ".ogg": true,

	// Fonts
	".ttf": true, ".otf": true, ".woff": true, ".woff2": true,

	// Machine learning model weights and graphs
	".onnx": true, ".safetensors": true, ".gguf": true, ".ggml": true,
	".pt": true, ".pth": true, ".ckpt": true, ".h5": true, ".hdf5": true,
	".pb": true, ".tflite": true, ".mlmodel": true, ".ort": true,

	// Serialized data and columnar stores
	".npy": true, ".npz": true, ".parquet": true, ".arrow": true,
	".feather": true, ".sqlite": true, ".sqlite3": true, ".db": true,
	".pkl": true, ".pickle": true, ".joblib": true,
	".msgpack": true, ".bson": true,

	// Other
	".pyc": true, ".class": true, ".o": true, ".a": true,
	".tiktoken": true,
}

// binarySniffBytes is the prefix window scanned for binary markers, matching
// the convention used by git and http.DetectContentType.
const binarySniffBytes = 512

// ASCII code points that the non-text scan treats specially.
const (
	asciiBackspace = 0x08
	asciiEscape    = 0x1b
	asciiSpace     = 0x20
	asciiDelete    = 0x7f
)

// Thresholds for the non-text scan applied when the sniff window holds no null
// byte. The two rules separate a strong signal from a weak one. Control
// characters are effectively absent from real text but pervasive in serialized
// framing such as ONNX, protobuf, and Parquet, so a small share of them is
// already decisive. Bytes that are not valid UTF-8 are weaker evidence,
// because legacy single-byte encodings such as Latin-1 produce them in
// ordinary prose, so they only decide the question at a much higher share.
// Each rule also needs an absolute minimum, so a short line carrying one stray
// control byte or a couple of accented bytes is not judged on a tiny sample.
const (
	binaryControlPercent = 5
	binaryControlMinimum = 4
	binaryNonTextPercent = 30
	binaryNonTextMinimum = 16
)

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

	return IsBinaryContent(path, buf[:n]), nil
}

// IsBinaryContent applies the same extension, null-byte, and non-text rules as
// IsBinaryFile to bytes that have already been read by a caller.
//
// Content counts as binary when the extension names a known binary format,
// when the first binarySniffBytes bytes contain a null byte, or when that same
// window fails the non-text scan. The scan classifies a byte as a control byte
// when it is a C0 control character other than tab, newline, carriage return,
// form feed, vertical tab, backspace, or escape, or when it is DEL. It
// classifies a byte as non-text when it is a control byte or begins a sequence
// that is not valid UTF-8. The window is binary when control bytes exceed
// binaryControlPercent of it, or when non-text bytes exceed
// binaryNonTextPercent of it, in each case above the matching absolute
// minimum. A multi-byte sequence truncated by the end of the window is not
// counted, so the window boundary never manufactures evidence of binary
// content.
func IsBinaryContent(path string, content []byte) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if binaryExtensions[ext] {
		return true
	}

	window := content[:min(len(content), binarySniffBytes)]
	if bytes.Contains(window, []byte{0}) {
		return true
	}

	counts := scanNonText(window)
	if counts.control >= binaryControlMinimum && counts.control*100 > len(window)*binaryControlPercent {
		return true
	}
	return counts.total >= binaryNonTextMinimum && counts.total*100 > len(window)*binaryNonTextPercent
}

// nonTextCounts summarizes bytes in a sniff window that text files do not
// normally carry.
type nonTextCounts struct {
	// control counts disallowed C0 control characters and DEL.
	control int
	// total counts control bytes plus bytes that cannot begin a valid UTF-8
	// sequence.
	total int
}

// scanNonText walks the sniff window once and reports both counts.
func scanNonText(window []byte) nonTextCounts {
	var counts nonTextCounts
	for index := 0; index < len(window); {
		current := window[index]
		if current < utf8.RuneSelf {
			if isControlByte(current) {
				counts.control++
				counts.total++
			}
			index++
			continue
		}

		if !utf8.FullRune(window[index:]) {
			// The window cut a multi-byte sequence in half. An invalid lead
			// byte still reports as a full (error) rune, so this only fires on
			// a truncated but otherwise well-formed prefix.
			break
		}

		_, size := utf8.DecodeRune(window[index:])
		if size == 1 {
			counts.total++
		}
		index += size
	}
	return counts
}

// isControlByte reports whether an ASCII byte is a control character that text
// files do not normally contain. Layout controls stay text, and so does the
// escape byte that introduces ANSI sequences in terminal transcripts.
func isControlByte(b byte) bool {
	switch b {
	case '\t', '\n', '\r', '\f', '\v', asciiBackspace, asciiEscape:
		return false
	}
	return b < asciiSpace || b == asciiDelete
}
