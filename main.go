package main

import (
	"fmt"
	"os"

	"git-remote-confluence/internal/remotehelper"
)

func main() {
	if err := remotehelper.Main(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "git-remote-confluence: %v\n", err)
		os.Exit(1)
	}
}
