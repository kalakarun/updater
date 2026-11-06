package main

import (
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(paths []string) error {
	if len(paths) == 0 {
		return errors.New("usage: cpguard-audit LINUX_BINARY...")
	}
	for _, name := range paths {
		binary, err := elf.Open(name)
		if err != nil {
			return err
		}
		for _, program := range binary.Progs {
			if program.Type == elf.PT_INTERP {
				binary.Close()
				return errors.New("binary has a dynamic interpreter: " + name)
			}
		}
		libraries, err := binary.ImportedLibraries()
		machine := binary.Machine.String()
		binary.Close()
		if err != nil {
			return err
		}
		if len(libraries) != 0 {
			return errors.New("binary imports shared libraries: " + name)
		}
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		hasher := sha256.New()
		size, copyErr := io.Copy(hasher, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"path": name, "machine": machine, "static": true, "size": size, "sha256": hex.EncodeToString(hasher.Sum(nil))}); err != nil {
			return err
		}
	}
	return nil
}
