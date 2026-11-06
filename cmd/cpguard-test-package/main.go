package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"cpguard/updater/internal/testpackage"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	flags := flag.NewFlagSet("cpguard-test-package", flag.ContinueOnError)
	input := flags.String("archive", "", "sample archive containing app/, plain tar or gzip")
	script := flags.String("script", "examples/test-package/run.sh", "copy-only test installer")
	output := flags.String("out", "", "new output tar.gz; existing files are never overwritten")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *input == "" || *output == "" || flags.NArg() != 0 {
		return errors.New("usage: cpguard-test-package --archive INPUT --out OUTPUT.tar.gz [--script run.sh]")
	}
	summary, err := testpackage.Build(*input, *script, *output)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(summary)
}
