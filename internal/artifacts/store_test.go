package artifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("source read failed") }

func TestContentAddressedRoundTripAndLimit(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first, err := store.Put(strings.NewReader("evidence"), 100)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Put(strings.NewReader("evidence"), 100)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest != second.Digest || first.Path != second.Path {
		t.Fatalf("content address is not stable: %+v %+v", first, second)
	}
	file, size, err := store.OpenVerified(first.Digest)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	content, _ := io.ReadAll(file)
	if string(content) != "evidence" || size != int64(len(content)) {
		t.Fatalf("content=%q size=%d", content, size)
	}
	cached, cachedSize, err := store.OpenVerified(first.Digest)
	if err != nil || cachedSize != size {
		t.Fatalf("cached verified open size=%d err=%v", cachedSize, err)
	}
	_ = cached.Close()
	if _, err := store.Put(strings.NewReader("too large"), 3); err == nil {
		t.Fatal("oversized artifact accepted")
	}
	if _, err := store.Put(failingReader{}, 100); err == nil {
		t.Fatal("source reader failure ignored")
	}
	if _, _, err := store.OpenVerified("sha256:../../secret"); err == nil {
		t.Fatal("unsafe digest accepted")
	}
}

func TestGarbageCollectionRespectsReferencesAndGrace(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	stored, err := store.Put(strings.NewReader("orphan candidate"), 1024)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(filepath.Join(root, stored.Path), old, old); err != nil {
		t.Fatal(err)
	}
	removed, err := store.GC(context.Background(), 24*time.Hour, func(context.Context, string) (bool, error) {
		return true, nil
	})
	if err != nil || removed != 0 {
		t.Fatalf("referenced artifact removed=%d err=%v", removed, err)
	}
	removed, err = store.GC(context.Background(), 24*time.Hour, func(_ context.Context, digest string) (bool, error) {
		if digest != stored.Digest {
			t.Fatalf("reference check digest=%q want=%q", digest, stored.Digest)
		}
		return false, nil
	})
	if err != nil || removed != 1 {
		t.Fatalf("orphan artifact removed=%d err=%v", removed, err)
	}
	if _, _, err := store.OpenVerified(stored.Digest); !os.IsNotExist(err) {
		t.Fatalf("collected artifact remained readable: %v", err)
	}
	if _, err := store.GC(context.Background(), 0, nil); err == nil {
		t.Fatal("invalid GC configuration accepted")
	}
}

func TestArtifactIntegrityAndHealthFailures(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Health(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(strings.NewReader("x"), 0); err == nil {
		t.Fatal("nonpositive size limit accepted")
	}
	for _, digest := range []string{"", "sha256:not-hex", "sha512:" + strings.Repeat("a", 64)} {
		if _, _, err := store.OpenVerified(digest); err == nil {
			t.Fatalf("invalid digest accepted: %q", digest)
		}
	}
	if _, _, err := store.OpenVerified("sha256:" + strings.Repeat("z", 64)); err == nil {
		t.Fatal("full-length non-hex digest accepted")
	}
	stored, err := store.Put(strings.NewReader("immutable"), 1024)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, stored.Path), []byte("corrupted"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.OpenVerified(stored.Digest); err == nil {
		t.Fatal("corrupted artifact passed verification")
	}
	if err := os.RemoveAll(filepath.Join(root, "tmp")); err != nil {
		t.Fatal(err)
	}
	if err := store.Health(); err == nil {
		t.Fatal("missing staging directory reported healthy")
	}
	if err := os.WriteFile(filepath.Join(root, "tmp"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Health(); err == nil {
		t.Fatal("staging file reported as healthy directory")
	}
	if _, err := store.Put(strings.NewReader("cannot stage"), 1024); err == nil {
		t.Fatal("artifact staged without a staging directory")
	}
}

func TestStoreRejectsInvalidRootAndGarbageCollectorErrors(t *testing.T) {
	root := t.TempDir()
	regular := filepath.Join(root, "regular")
	if err := os.WriteFile(regular, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(regular); err == nil {
		t.Fatal("regular file accepted as artifact root")
	}
	blockedSHA := t.TempDir()
	if err := os.WriteFile(filepath.Join(blockedSHA, "sha256"), []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(blockedSHA); err == nil {
		t.Fatal("invalid content directory accepted")
	}
	blockedTemp := t.TempDir()
	if err := os.Mkdir(filepath.Join(blockedTemp, "sha256"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blockedTemp, "tmp"), []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(blockedTemp); err == nil {
		t.Fatal("invalid staging directory accepted")
	}
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	stored, err := store.Put(strings.NewReader("gc callback"), 1024)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(filepath.Join(root, stored.Path), old, old); err != nil {
		t.Fatal(err)
	}
	fresh, err := store.Put(strings.NewReader("fresh"), 1024)
	if err != nil {
		t.Fatal(err)
	}
	callbacks := 0
	if removed, err := store.GC(context.Background(), 24*time.Hour, func(context.Context, string) (bool, error) {
		callbacks++
		return true, nil
	}); err != nil || removed != 0 || callbacks != 1 {
		t.Fatalf("grace filtering removed=%d callbacks=%d err=%v", removed, callbacks, err)
	}
	if _, _, err := store.OpenVerified(fresh.Digest); err != nil {
		t.Fatalf("fresh artifact removed by GC: %v", err)
	}
	sentinel := errors.New("reference store unavailable")
	if _, err := store.GC(context.Background(), time.Hour, func(context.Context, string) (bool, error) {
		return false, sentinel
	}); !errors.Is(err, sentinel) {
		t.Fatalf("reference callback error=%v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.GC(cancelled, time.Hour, func(context.Context, string) (bool, error) {
		return false, nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled GC error=%v", err)
	}
	if _, _, err := store.OpenVerified("sha256:" + strings.Repeat("a", 64)); !os.IsNotExist(err) {
		t.Fatalf("missing valid digest error=%v", err)
	}
}

func TestStoreFailsClosedOnInvalidContentAddressLayout(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := os.RemoveAll(filepath.Join(root, "sha256")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sha256"), []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(strings.NewReader("cannot publish"), 1024); err == nil {
		t.Fatal("artifact published through invalid content layout")
	}
}

func TestPutRejectsInvalidExistingContentAddress(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	content := "must remain immutable"
	digestBytes := sha256.Sum256([]byte(content))
	digest := hex.EncodeToString(digestBytes[:])
	path := filepath.Join(root, "sha256", digest[:2], digest)
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(strings.NewReader(content), 1024); err == nil {
		t.Fatal("directory at existing content address was accepted")
	}
}

func TestOpenVerifiedRejectsDirectoryAtDigestPath(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	digest := strings.Repeat("b", 64)
	if err := os.MkdirAll(filepath.Join(root, "sha256", digest[:2], digest), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.OpenVerified("sha256:" + digest); err == nil {
		t.Fatal("directory at digest path passed content verification")
	}
}

func TestGarbageCollectionFailsAfterStoreClose(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GC(context.Background(), time.Hour, func(context.Context, string) (bool, error) {
		return false, nil
	}); err == nil {
		t.Fatal("closed artifact store allowed garbage collection")
	}
}

func TestConcurrentIdenticalPutPublishesOneVerifiedBlob(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	const writers = 16
	results := make(chan Stored, writers)
	errs := make(chan error, writers)
	var group sync.WaitGroup
	for range writers {
		group.Add(1)
		go func() {
			defer group.Done()
			stored, err := store.Put(strings.NewReader("same immutable evidence"), 1024)
			results <- stored
			errs <- err
		}()
	}
	group.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var digest string
	created := 0
	for stored := range results {
		if digest == "" {
			digest = stored.Digest
		}
		if stored.Digest != digest {
			t.Fatalf("digest = %q, want %q", stored.Digest, digest)
		}
		if stored.Created {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("created count = %d, want exactly one atomic publisher", created)
	}
	file, _, err := store.OpenVerified(digest)
	if err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
}
