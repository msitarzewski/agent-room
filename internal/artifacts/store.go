package artifacts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"sync"
	"time"
)

type Stored struct {
	Digest  string
	Size    int64
	Path    string
	Created bool
}

type Store struct {
	root     *os.Root
	mu       sync.Mutex
	verified map[string]verification
}

type verification struct {
	size    int64
	modTime time.Time
}

func Open(directory string) (*Store, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	if err := root.MkdirAll("sha256", 0o750); err != nil {
		_ = root.Close()
		return nil, err
	}
	if err := root.MkdirAll("tmp", 0o750); err != nil {
		_ = root.Close()
		return nil, err
	}
	return &Store{root: root, verified: make(map[string]verification)}, nil
}

func (s *Store) Close() error { return s.root.Close() }

func (s *Store) Health() error {
	for _, directory := range []string{"sha256", "tmp"} {
		info, err := s.root.Stat(directory)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("%s is not a directory", directory)
		}
	}
	return nil
}

func (s *Store) Put(reader io.Reader, maximum int64) (Stored, error) {
	if maximum <= 0 {
		return Stored{}, errors.New("artifact size limit must be positive")
	}
	tempName := "tmp/" + randomName()
	file, err := s.root.OpenFile(tempName, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o640)
	if err != nil {
		return Stored{}, err
	}
	keep := false
	defer func() {
		_ = file.Close()
		if !keep {
			_ = s.root.Remove(tempName)
		}
	}()
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(reader, maximum+1))
	if err != nil {
		return Stored{}, err
	}
	if size > maximum {
		return Stored{}, errors.New("artifact exceeds size limit")
	}
	digestHex := hex.EncodeToString(hash.Sum(nil))
	directory := "sha256/" + digestHex[:2]
	finalName := directory + "/" + digestHex
	if err := s.root.MkdirAll(directory, 0o750); err != nil {
		return Stored{}, err
	}
	if _, err := s.root.Stat(finalName); err == nil {
		existing, _, verifyErr := s.OpenVerified("sha256:" + digestHex)
		if verifyErr != nil {
			return Stored{}, verifyErr
		}
		_ = existing.Close()
		keep = true
		_ = s.root.Remove(tempName)
		return Stored{Digest: "sha256:" + digestHex, Size: size, Path: finalName}, nil
	}
	if err := file.Sync(); err != nil {
		return Stored{}, err
	}
	if err := file.Close(); err != nil {
		return Stored{}, err
	}
	if err := s.root.Link(tempName, finalName); err != nil {
		// Hard-link creation is an atomic no-overwrite publish. Another writer
		// of identical content may have won, but its final content address is
		// accepted only after verification.
		if _, _, verifyErr := s.OpenVerified("sha256:" + digestHex); verifyErr == nil {
			keep = true
			_ = s.root.Remove(tempName)
			return Stored{Digest: "sha256:" + digestHex, Size: size, Path: finalName}, nil
		}
		return Stored{}, err
	}
	if err := s.root.Remove(tempName); err != nil {
		return Stored{}, err
	}
	keep = true
	return Stored{Digest: "sha256:" + digestHex, Size: size, Path: finalName, Created: true}, nil
}

func (s *Store) OpenVerified(digest string) (*os.File, int64, error) {
	if !strings.HasPrefix(digest, "sha256:") || len(digest) != len("sha256:")+64 {
		return nil, 0, errors.New("invalid artifact digest")
	}
	hexDigest := strings.TrimPrefix(digest, "sha256:")
	if _, err := hex.DecodeString(hexDigest); err != nil {
		return nil, 0, errors.New("invalid artifact digest")
	}
	file, err := s.root.Open("sha256/" + hexDigest[:2] + "/" + hexDigest)
	if err != nil {
		return nil, 0, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, 0, err
	}
	s.mu.Lock()
	cached, ok := s.verified[digest]
	s.mu.Unlock()
	if ok && cached.size == info.Size() && cached.modTime.Equal(info.ModTime()) {
		return file, info.Size(), nil
	}
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		file.Close()
		return nil, 0, err
	}
	if actual := hex.EncodeToString(hash.Sum(nil)); actual != hexDigest {
		file.Close()
		return nil, 0, fmt.Errorf("artifact digest verification failed")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		return nil, 0, err
	}
	s.mu.Lock()
	s.verified[digest] = verification{size: size, modTime: info.ModTime()}
	s.mu.Unlock()
	return file, size, nil
}

// GC removes old content-addressed blobs only after the caller confirms that
// no canonical artifact resource references the digest. This provides
// recovery for uploads that reached disk but whose database command failed.
func (s *Store) GC(ctx context.Context, grace time.Duration, referenced func(context.Context, string) (bool, error)) (int, error) {
	if grace <= 0 || referenced == nil {
		return 0, errors.New("positive grace period and reference check are required")
	}
	now := time.Now()
	removed := 0
	err := fs.WalkDir(s.root.FS(), "sha256", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if len(name) != 64 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if now.Sub(info.ModTime()) < grace {
			return nil
		}
		digest := "sha256:" + name
		used, err := referenced(ctx, digest)
		if err != nil {
			return err
		}
		if used {
			return nil
		}
		if err := s.root.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		s.mu.Lock()
		delete(s.verified, digest)
		s.mu.Unlock()
		removed++
		return nil
	})
	return removed, err
}

func randomName() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}
