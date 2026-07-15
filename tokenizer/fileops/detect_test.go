package fileops

import "testing"

func TestIsBinaryContentPreservesExtensionAndPrefixRules(t *testing.T) {
	contentAfterPrefix := make([]byte, binarySniffBytes+1)
	for index := range contentAfterPrefix {
		contentAfterPrefix[index] = 'a'
	}
	contentAfterPrefix[binarySniffBytes] = 0
	tests := []struct {
		name    string
		path    string
		content []byte
		want    bool
	}{
		{name: "binary extension", path: "image.png", content: []byte("plain text"), want: true},
		{name: "null in prefix", path: "data.txt", content: []byte{'a', 0, 'b'}, want: true},
		{name: "ordinary text", path: "readme.md", content: []byte("ordinary text\n"), want: false},
		{name: "null after prefix", path: "large.txt", content: contentAfterPrefix, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsBinaryContent(test.path, test.content); got != test.want {
				t.Fatalf("IsBinaryContent(%q) = %t, want %t", test.path, got, test.want)
			}
		})
	}
}
