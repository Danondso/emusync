package hasher

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// HashFile returns the hex-encoded SHA-256 hash of a file.
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hashing %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// HashDirectory walks a directory and returns a map of relative path to SHA-256 hash
// for all regular files. Hashing is done concurrently.
func HashDirectory(dir string) (map[string]string, error) {
	var mu sync.Mutex
	result := make(map[string]string)

	type job struct {
		absPath string
		relPath string
	}

	jobs := make(chan job, 100)
	var wg sync.WaitGroup
	errs := make(chan error, 1)

	// Start workers
	workers := runtime.NumCPU()
	if workers < 2 {
		workers = 2
	}
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				hash, err := HashFile(j.absPath)
				if err != nil {
					select {
					case errs <- err:
					default:
					}
					continue
				}
				mu.Lock()
				result[j.relPath] = hash
				mu.Unlock()
			}
		}()
	}

	// Walk directory and enqueue jobs
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		jobs <- job{absPath: path, relPath: rel}
		return nil
	})
	close(jobs)
	wg.Wait()

	if err != nil {
		return nil, fmt.Errorf("walking %s: %w", dir, err)
	}

	select {
	case e := <-errs:
		return nil, e
	default:
	}

	return result, nil
}
