package main

import (
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"cpguard/updater/internal/durable"
	"cpguard/updater/internal/manifest"
	"cpguard/updater/internal/publisher"
	"golang.org/x/term"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func passphrase(confirm bool) ([]byte, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil, errors.New("passphrase input requires an interactive terminal")
	}
	fmt.Fprint(os.Stderr, "Key passphrase: ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, err
	}
	if confirm {
		fmt.Fprint(os.Stderr, "Repeat passphrase: ")
		repeated, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		defer clear(repeated)
		if err != nil || string(password) != string(repeated) {
			clear(password)
			return nil, errors.New("passphrases do not match")
		}
	}
	return password, nil
}

func run(arguments []string) error {
	if len(arguments) == 0 {
		return errors.New("usage: cpguard-sign keygen|make|renew [flags]")
	}
	command := arguments[0]
	if command != "keygen" && command != "make" && command != "renew" {
		return errors.New("unknown signing command")
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	output := flags.String("out", "", "new output path; existing files are never overwritten")
	publicPath := flags.String("public", "", "public key output path for keygen")
	keyPath := flags.String("key", "", "age-encrypted Ed25519 private key")
	archivePath := flags.String("archive", "", "single universal tar.gz archive")
	address := flags.String("url", "", "HTTPS archive URL")
	version := flags.String("version", "", "application version major.minor.patch")
	sequence := flags.Uint64("sequence", 0, "positive, increasing publisher sequence")
	input := flags.String("in", "", "previous update_check.json for renew")
	validity := flags.Duration("valid-for", 7*24*time.Hour, "metadata validity, at most 168h")
	if err := flags.Parse(arguments[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *output == "" {
		return errors.New("--out is required; unexpected positional arguments are forbidden")
	}
	if command == "keygen" {
		if *publicPath == "" || *publicPath == *output {
			return errors.New("distinct --out and --public paths required")
		}
		password, err := passphrase(true)
		if err != nil {
			return err
		}
		defer clear(password)
		encrypted, public, err := publisher.Generate(string(password))
		if err != nil {
			return err
		}
		if err := publisher.WriteNew(*output, encrypted); err != nil {
			return err
		}
		encoded := base64.StdEncoding.EncodeToString(public)
		if err := publisher.WriteNew(*publicPath, []byte(encoded+"\n")); err != nil {
			return err
		}
		fmt.Println(encoded)
		return nil
	}
	if *sequence == 0 || *validity < time.Second || *validity > 7*24*time.Hour || *validity%time.Second != 0 {
		return errors.New("positive --sequence and whole-second validity of 1s..168h required")
	}
	if command == "make" && (*archivePath == "" || *address == "" || *version == "") {
		return errors.New("make requires --archive, --url, and --version")
	}
	if command == "renew" && *input == "" {
		return errors.New("renew requires --in")
	}
	encrypted, err := durable.Read(*keyPath, 65536)
	if err != nil {
		return err
	}
	password, err := passphrase(false)
	if err != nil {
		return err
	}
	defer clear(password)
	private, err := publisher.Decrypt(encrypted, string(password))
	if err != nil {
		return err
	}
	defer clear(private)
	now := time.Now().UTC().Truncate(time.Second)
	var encoded []byte
	if command == "make" {
		payload, err := publisher.Make(*archivePath, *address, *version, *sequence, now, *validity)
		if err != nil {
			return err
		}
		encoded, err = manifest.Sign(payload, private, now)
		if err != nil {
			return err
		}
	} else {
		previous, err := durable.Read(*input, manifest.MaxBytes)
		if err != nil {
			return err
		}
		encoded, err = publisher.Renew(previous, private, *sequence, now, *validity)
		if err != nil {
			return err
		}
	}
	return publisher.WriteNew(*output, encoded)
}
