// Package store reads and writes the plain-text PR list files used by prowl.
// Format is one PR URL per line; blank lines and lines beginning with `#` are
// ignored. A historical "<tag>|<url>" line format is also tolerated for reads.
package store

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Store is a pair of plain-text files: an active list and a reviewed list.
type Store struct {
	ActivePath   string
	ReviewedPath string
}

// New constructs a Store and ensures both files exist. It also migrates
// pre-existing legacy files (prs-unmerged.txt, prs-merged.txt, prs-closed.txt)
// into the new active/reviewed layout.
func New(active, reviewed string) (*Store, error) {
	s := &Store{ActivePath: active, ReviewedPath: reviewed}
	if err := s.ensure(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) ensure() error {
	if err := os.MkdirAll(filepath.Dir(s.ActivePath), 0o755); err != nil {
		return fmt.Errorf("mkdir active: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.ReviewedPath), 0o755); err != nil {
		return fmt.Errorf("mkdir reviewed: %w", err)
	}
	if err := s.migrate(); err != nil {
		return err
	}
	if err := touch(s.ActivePath); err != nil {
		return err
	}
	return touch(s.ReviewedPath)
}

func (s *Store) migrate() error {
	dir := filepath.Dir(s.ActivePath)
	legacyUnmerged := filepath.Join(dir, "prs-unmerged.txt")
	legacyMerged := filepath.Join(dir, "prs-merged.txt")
	legacyClosed := filepath.Join(dir, "prs-closed.txt")

	if _, err := os.Stat(s.ActivePath); errors.Is(err, os.ErrNotExist) {
		if _, err := os.Stat(legacyUnmerged); err == nil {
			if err := os.Rename(legacyUnmerged, s.ActivePath); err != nil {
				return fmt.Errorf("migrate unmerged: %w", err)
			}
		}
	}
	for _, legacy := range []string{legacyMerged, legacyClosed} {
		if _, err := os.Stat(legacy); err != nil {
			continue
		}
		if err := appendFile(s.ReviewedPath, legacy); err != nil {
			return err
		}
		if err := os.Remove(legacy); err != nil {
			return fmt.Errorf("remove legacy %s: %w", legacy, err)
		}
	}
	return nil
}

func appendFile(dst, src string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	f, err := os.OpenFile(dst, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", dst, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}

func touch(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	return f.Close()
}

// Active returns the URLs stored in the active list (in file order).
func (s *Store) Active() ([]string, error) { return ReadURLs(s.ActivePath) }

// Reviewed returns the URLs stored in the reviewed list (in file order).
func (s *Store) Reviewed() ([]string, error) { return ReadURLs(s.ReviewedPath) }

// ReadURLs parses a list file and returns the URLs in document order.
func ReadURLs(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	var urls []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if u := parseLine(scanner.Text()); u != "" {
			urls = append(urls, u)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}
	return urls, nil
}

// parseLine trims whitespace, drops comments/blank lines, and strips the
// legacy "<tag>|" prefix.
func parseLine(line string) string {
	trim := strings.TrimSpace(line)
	if trim == "" || strings.HasPrefix(trim, "#") {
		return ""
	}
	if i := strings.Index(trim, "|"); i >= 0 {
		trim = strings.TrimSpace(trim[i+1:])
	}
	return trim
}

// Add appends a URL to the active list. Returns false if the URL is already
// tracked in either list.
func (s *Store) Add(url string) (bool, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return false, errors.New("empty URL")
	}
	for _, path := range []string{s.ActivePath, s.ReviewedPath} {
		existing, err := ReadURLs(path)
		if err != nil {
			return false, err
		}
		for _, u := range existing {
			if u == url {
				return false, nil
			}
		}
	}
	f, err := os.OpenFile(s.ActivePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return false, fmt.Errorf("open active: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := fmt.Fprintln(f, url); err != nil {
		return false, fmt.Errorf("write active: %w", err)
	}
	return true, nil
}

// Remove deletes the URL from both lists. Returns the number of removed rows.
func (s *Store) Remove(url string) (int, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return 0, nil
	}
	total := 0
	for _, path := range []string{s.ActivePath, s.ReviewedPath} {
		n, err := removeFromFile(path, map[string]struct{}{url: {}})
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

// MoveActiveToReviewed moves the given URLs from the active list to the
// reviewed list, preserving line order. URLs that aren't present in active
// are silently ignored.
func (s *Store) MoveActiveToReviewed(urls []string) (int, error) {
	if len(urls) == 0 {
		return 0, nil
	}
	target := stringSet(urls)
	moved, err := moveBetweenFiles(s.ActivePath, s.ReviewedPath, target)
	if err != nil {
		return moved, err
	}
	return moved, nil
}

func moveBetweenFiles(src, dst string, target map[string]struct{}) (int, error) {
	srcLines, err := readLines(src)
	if err != nil {
		return 0, err
	}
	dstFile, err := os.OpenFile(dst, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", dst, err)
	}
	defer func() { _ = dstFile.Close() }()

	var kept []string
	moved := 0
	for _, line := range srcLines {
		url := parseLine(line)
		if url == "" {
			kept = append(kept, line)
			continue
		}
		if _, ok := target[url]; ok {
			if _, err := fmt.Fprintln(dstFile, line); err != nil {
				return moved, fmt.Errorf("append %s: %w", dst, err)
			}
			moved++
			continue
		}
		kept = append(kept, line)
	}
	if err := writeLines(src, kept); err != nil {
		return moved, err
	}
	return moved, nil
}

func removeFromFile(path string, target map[string]struct{}) (int, error) {
	lines, err := readLines(path)
	if err != nil {
		return 0, err
	}
	removed := 0
	var kept []string
	for _, line := range lines {
		url := parseLine(line)
		if _, ok := target[url]; ok && url != "" {
			removed++
			continue
		}
		kept = append(kept, line)
	}
	if removed == 0 {
		return 0, nil
	}
	if err := writeLines(path, kept); err != nil {
		return removed, err
	}
	return removed, nil
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	var out []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		out = append(out, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}
	return out, nil
}

func writeLines(path string, lines []string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".prowl-*")
	if err != nil {
		return fmt.Errorf("temp file: %w", err)
	}
	w := bufio.NewWriter(tmp)
	for _, line := range lines {
		if _, err := fmt.Fprintln(w, line); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
			return fmt.Errorf("write tmp: %w", err)
		}
	}
	if err := w.Flush(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("flush tmp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("close tmp: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("rename %s: %w", path, err)
	}
	return nil
}

func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, v := range values {
		if v != "" {
			out[v] = struct{}{}
		}
	}
	return out
}
