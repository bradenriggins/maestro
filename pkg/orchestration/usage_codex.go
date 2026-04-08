package orchestration

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"maestro/pkg/accounts"
)

// codexRateLimits represents the parsed output from Codex rate-limit queries.
type codexRateLimits struct {
	UsedPercent   float64 `json:"used_percent"`
	WindowMinutes int     `json:"window_minutes"`
	ResetsAt      int64   `json:"resets_at"`
}

// codexCacheEntry is the on-disk cache format.
type codexCacheEntry struct {
	UsedPercent   float64 `json:"used_percent"`
	ResetsAt      int64   `json:"resets_at"`
	WindowMinutes int     `json:"window_minutes"`
	CachedAt      int64   `json:"cached_at"`
}

// codexSessionEvent is the minimal shape of a JSONL event line.
type codexSessionEvent struct {
	Type    string `json:"type"`
	Payload struct {
		Type       string `json:"type"`
		RateLimits *struct {
			Primary *struct {
				UsedPercent     float64 `json:"used_percent"`
				WindowMinutes   int     `json:"window_minutes"`
				ResetsInSeconds float64 `json:"resets_in_seconds"`
			} `json:"primary"`
		} `json:"rate_limits"`
	} `json:"payload"`
}

const (
	codexCacheTTL  = 60 * time.Second
	codexMaxFiles  = 3
	codexMaxLines  = 500
	codexChunkSize = 4096
)

// collectCodexUsage reads rate limit data for a Codex account by scanning the
// most recent session JSONL files written by the Codex CLI.
// Returns nil when no data is available (no sessions dir, no JSONL files, or
// no token_count event with rate_limits found in the 3 most recent files).
func collectCodexUsage(accountName, configDir string) *codexRateLimits {
	base, err := accounts.ConductorDir()
	if err != nil {
		return nil
	}

	// --- Cache check ---
	usageCacheDir := filepath.Join(base, "usage")
	usagePath := filepath.Join(usageCacheDir, accountName+".json")

	if data, err := os.ReadFile(usagePath); err == nil && len(data) > 0 {
		var entry codexCacheEntry
		if json.Unmarshal(data, &entry) == nil && entry.CachedAt > 0 {
			age := time.Since(time.Unix(entry.CachedAt, 0))
			if age < codexCacheTTL {
				return &codexRateLimits{
					UsedPercent:   entry.UsedPercent,
					WindowMinutes: entry.WindowMinutes,
					ResetsAt:      entry.ResetsAt,
				}
			}
		}
	}

	// --- Scan session files ---
	sessionsDir := filepath.Join(configDir, "sessions")
	if _, err := os.Stat(sessionsDir); os.IsNotExist(err) {
		return nil
	}

	files, err := findJSONLFiles(sessionsDir)
	if err != nil || len(files) == 0 {
		return nil
	}

	// Limit to the 3 most recent files.
	if len(files) > codexMaxFiles {
		files = files[:codexMaxFiles]
	}

	for _, f := range files {
		result := scanJSONLReverse(f)
		if result != nil {
			// Write cache.
			if mkErr := os.MkdirAll(usageCacheDir, 0700); mkErr == nil {
				entry := codexCacheEntry{
					UsedPercent:   result.UsedPercent,
					ResetsAt:      result.ResetsAt,
					WindowMinutes: result.WindowMinutes,
					CachedAt:      time.Now().Unix(),
				}
				_ = AtomicWriteJSON(usagePath, entry)
			}
			return result
		}
	}

	return nil
}

// findJSONLFiles returns all .jsonl files under dir, sorted newest-first by
// modification time.
func findJSONLFiles(dir string) ([]string, error) {
	type fileInfo struct {
		path    string
		modTime time.Time
	}

	var files []fileInfo
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".jsonl" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		files = append(files, fileInfo{path: path, modTime: info.ModTime()})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})

	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.path
	}
	return paths, nil
}

// scanJSONLReverse reads up to codexMaxLines lines from the end of a JSONL file,
// parsing each as a Codex session event. Returns the first token_count event
// with non-null rate_limits, or nil if none found.
func scanJSONLReverse(path string) *codexRateLimits {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	size, err := f.Seek(0, io.SeekEnd)
	if err != nil || size == 0 {
		return nil
	}

	// Read the file in reverse chunks, collecting complete lines.
	lines, err := readLastLines(f, size, codexMaxLines)
	if err != nil {
		return nil
	}

	// Iterate lines in reverse order (last line first).
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}
		var event codexSessionEvent
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}
		if event.Type != "event_msg" {
			continue
		}
		if event.Payload.Type != "token_count" {
			continue
		}
		if event.Payload.RateLimits == nil || event.Payload.RateLimits.Primary == nil {
			continue
		}
		p := event.Payload.RateLimits.Primary
		return &codexRateLimits{
			UsedPercent:   p.UsedPercent,
			WindowMinutes: p.WindowMinutes,
			ResetsAt:      time.Now().Unix() + int64(p.ResetsInSeconds),
		}
	}
	return nil
}

// readLastLines reads up to maxLines complete lines from the end of the file,
// returning them in forward order (earliest first).
func readLastLines(f *os.File, size int64, maxLines int) ([][]byte, error) {
	var collected [][]byte
	pos := size
	// tail holds a partial line fragment carried from the right end of the
	// current chunk into the next (leftward) chunk.
	var tail []byte

	for pos > 0 && len(collected) < maxLines {
		chunkSize := int64(codexChunkSize)
		if pos < chunkSize {
			chunkSize = pos
		}
		pos -= chunkSize

		if _, err := f.Seek(pos, io.SeekStart); err != nil {
			return nil, err
		}

		chunk := make([]byte, chunkSize)
		n, err := io.ReadFull(f, chunk)
		if err != nil && err != io.ErrUnexpectedEOF {
			return nil, err
		}
		chunk = chunk[:n]

		// Prepend chunk to any partial line fragment from the previous (rightward) iteration.
		// combined = <this chunk><right-fragment>
		combined := append(chunk, tail...) //nolint:gocritic // intentional: chunk is not reused after this point

		// Split on newlines. We want every complete line that starts within
		// `combined`. Because combined may itself start mid-line (when pos > 0),
		// the first segment is potentially a partial line that continues further
		// to the left — we will carry it as the new tail.
		scanner := bufio.NewScanner(bytes.NewReader(combined))
		// Increase the scanner buffer to handle JSONL lines up to 1 MiB, which
		// is large enough for any realistic Codex session event. Without this,
		// the default 64 KiB limit silently drops oversized lines.
		const maxTokenSize = 1 << 20 // 1 MiB
		scanner.Buffer(make([]byte, bufio.MaxScanTokenSize), maxTokenSize)
		var parts [][]byte
		for scanner.Scan() {
			b := scanner.Bytes()
			cp := make([]byte, len(b))
			copy(cp, b)
			parts = append(parts, cp)
		}
		// Ignore scanner.Err() — a line that is still too large after our
		// generous buffer would simply produce no token; nothing we can do.

		// If combined doesn't end with a newline, the LAST segment (rightmost)
		// was already accounted for by the previous iteration's tail and is a
		// complete right-hand boundary — no adjustment needed.
		//
		// If combined doesn't START with a newline and pos > 0, the FIRST
		// segment is a partial line that continues to the left. Carry it as
		// the new tail; the remaining parts are complete lines.
		if pos > 0 && len(combined) > 0 && combined[0] != '\n' && len(parts) > 0 {
			tail = make([]byte, len(parts[0]))
			copy(tail, parts[0])
			parts = parts[1:]
		} else {
			tail = nil
		}

		// Parts are in forward order within this chunk; prepend them so that
		// collected remains in forward (file) order overall.
		newCollected := make([][]byte, 0, len(parts)+len(collected))
		newCollected = append(newCollected, parts...)
		newCollected = append(newCollected, collected...)
		collected = newCollected
	}

	// If there's a remaining tail (the very first line of the file had no
	// leading newline and we ran out of file to the left), prepend it.
	if len(tail) > 0 {
		collected = append([][]byte{tail}, collected...)
	}

	// Trim to maxLines (keep the last maxLines).
	if len(collected) > maxLines {
		collected = collected[len(collected)-maxLines:]
	}

	return collected, nil
}
