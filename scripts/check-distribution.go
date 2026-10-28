//go:build ignore

package main

import (
	"fmt"
	"os"

	"gcode/scripts/distribution"
)

func main() {
	config, err := distribution.Load("mygo.json")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("Distribution: UI %s; Windows %s; macOS %s; identifier %s\n", config.Name, config.WindowsInstallDirectory(), config.BundleFile(), config.Identifier)
}
