package security_test

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/msitarzewski/agent-room/internal/artifacts"
)

func TestConcurrentArtifactPutIsContentAddressedAndVerifiable(t *testing.T) {
	root := t.TempDir()
	store, err := artifacts.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	const workers = 32
	results := make(chan artifacts.Stored, workers)
	failures := make(chan error, workers)
	var group sync.WaitGroup
	for index := 0; index < workers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			stored, putErr := store.Put(strings.NewReader("shared immutable evidence"), 1024)
			if putErr != nil {
				failures <- putErr
				return
			}
			results <- stored
		}()
	}
	group.Wait()
	close(results)
	close(failures)
	for failure := range failures {
		t.Error(failure)
	}

	var expected artifacts.Stored
	for stored := range results {
		if expected.Digest == "" {
			expected = stored
			continue
		}
		if stored.Digest != expected.Digest || stored.Path != expected.Path {
			t.Fatalf("concurrent content address mismatch: %+v != %+v", stored, expected)
		}
	}
	file, size, err := store.OpenVerified(expected.Digest)
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(file)
	_ = file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "shared immutable evidence" || size != int64(len(content)) {
		t.Fatalf("stored content=%q size=%d", content, size)
	}

	if err := os.WriteFile(filepath.Join(root, expected.Path), []byte(fmt.Sprintf("%0*d", len(content), 0)), 0o640); err != nil {
		t.Fatal(err)
	}
	if file, _, err = store.OpenVerified(expected.Digest); err == nil {
		_ = file.Close()
		t.Fatal("mutated artifact passed digest verification")
	}
}
