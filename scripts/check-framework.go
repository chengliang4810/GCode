//go:build ignore

package main

import (
	"flag"
	"fmt"
	"os"

	"gcode/scripts/frameworkpin"
)

func main() {
	ldflags := flag.Bool("ldflags", false, "Print the verified framework linker stamp only")
	fingerprint := flag.Bool("fingerprint", false, "Print the current tree fingerprint for patch review (does not verify)")
	fingerprintPath := flag.String("fingerprint-path", "third_party/mygo", "Source tree for fingerprint diagnostics only")
	flag.Parse()
	if *fingerprint {
		if *ldflags {
			fmt.Fprintln(os.Stderr, "fingerprint and ldflags are mutually exclusive")
			os.Exit(1)
		}
		digest, count, err := frameworkpin.TreeHash(*fingerprintPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("%s %d\n", digest, count)
		return
	}
	lock, err := frameworkpin.VerifySource(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *ldflags {
		fmt.Println(frameworkpin.LinkerFlags(lock))
		return
	}
	fmt.Printf("Verified MyGo %s + %s: %s (%d files)\n", lock.UpstreamVersion, lock.Revision, lock.TreeSHA256, lock.FileCount)
}
