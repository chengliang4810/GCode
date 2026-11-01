//go:build ignore

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"gcode/scripts/updatefeed"
)

func main() {
	root := flag.String("root", "build", "Release directory")
	targets := flag.String("targets", "windows/amd64", "Comma-separated targets")
	checkKey := flag.Bool("check-key", false, "Validate the release signing key before building")
	flag.Parse()
	config, err := updatefeed.Load("mygo.json")
	if err == nil {
		if *checkKey {
			err = config.CheckSigningKey(os.Getenv("MYGO_UPDATER_PRIVATE_KEY"))
		} else {
			err = config.Verify(*root, strings.Split(*targets, ","))
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Signed update verification passed.")
}
