package installer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProcessGroupTimeout(t *testing.T) {
	directory := t.TempDir()
	script := "#!/bin/sh\n(sleep 1; printf escaped > escaped) &\nwait\n"
	if err := os.WriteFile(filepath.Join(directory, "run.sh"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if err := (Runner{Timeout: 100 * time.Millisecond}).Run(context.Background(), directory); err == nil {
		t.Fatal("timeout accepted")
	}
	if time.Since(started) > 3*time.Second {
		t.Fatal("installer did not terminate promptly")
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(directory, "escaped")); !os.IsNotExist(err) {
		t.Fatal("installer child survived timeout")
	}
}

func TestOutputBound(t *testing.T) {
	output := &bounded{remaining: 4}
	written, err := output.Write([]byte("123456"))
	if err != nil || written != 6 || !output.truncated || string(output.output) != "1234" {
		t.Fatal(output, written, err)
	}
	_, _ = output.Write([]byte(strings.Repeat("x", 1000)))
	if len(output.output) != 4 {
		t.Fatal("unbounded output")
	}
}
