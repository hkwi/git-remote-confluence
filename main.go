package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/hkwi/git-remote-confluence/internal/confluence"
	"github.com/hkwi/git-remote-confluence/internal/remotehelper"
)

const appName = "git-remote-confluence"

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	args := os.Args[1:]
	if len(args) == 1 {
		switch args[0] {
		case "version", "--version", "-version":
			fmt.Print(versionOutput())
			return
		}
	}

	confluence.SetUserAgentVersion(releaseVersion())
	if err := remotehelper.Main(args, os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", appName, err)
		os.Exit(1)
	}
}

func versionOutput() string {
	return fmt.Sprintf("%s %s\ncommit: %s\nbuilt: %s\n", appName, releaseVersion(), commit, date)
}

func releaseVersion() string {
	if version != "dev" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return version
	}
	return info.Main.Version
}
