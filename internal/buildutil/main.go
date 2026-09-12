package main

import (
	"os"
	"os/exec"
	"strings"
	"time"

	buildutil "github.com/Obedience-Corp/build-util"
)

func main() {
	buildutil.Run(os.Args[1:], buildutil.BuildConfig{
		BinaryName:  "tcount",
		MainPath:    "./cmd/tcount",
		SectionName: "tcount",
		Icon:        "🔧",
		LDFlags:     ldflags,
		TestTimeout: 30 * time.Second,
		CleanPatterns: []string{
			"bin/",
			"*.test",
			"*.exe",
			"coverage.out",
			"coverage.html",
			".test-*",
			"*.tmp",
		},
		SkipDockerCleanup: true,
	})
}

func ldflags() string {
	version := "dev"
	if out, err := exec.Command("git", "describe", "--tags", "--always", "--dirty").Output(); err == nil {
		version = strings.TrimPrefix(strings.TrimSpace(string(out)), "v")
	}
	return "-s -w -X main.version=" + version
}
