package sources

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// readArtifactJSONLines decodes only complete JSONL records and never reads
// more than DefaultScanFileBytes from one artifact. For an oversized artifact
// it favors the tail so recent events remain available and drops a partial
// leading record.
func readArtifactJSONLines(path string) (lines []jsonLine, parseError bool, truncated bool, unreadable bool) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false, false, true
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, false, false, true
	}
	readSize := info.Size()
	start := int64(0)
	if readSize > DefaultScanFileBytes {
		truncated = true
		readSize = DefaultScanFileBytes
		start = info.Size() - readSize
	}
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return nil, false, truncated, true
	}
	contents, err := io.ReadAll(io.LimitReader(file, readSize))
	if err != nil {
		return nil, false, truncated, true
	}
	if start > 0 {
		if boundary := bytes.IndexByte(contents, '\n'); boundary >= 0 {
			contents = contents[boundary+1:]
		} else {
			return nil, false, true, false
		}
	}

	parts := bytes.Split(contents, []byte{'\n'})
	lines = make([]jsonLine, 0, len(parts))
	for sequence, raw := range parts {
		raw = bytes.TrimSpace(raw)
		if len(raw) == 0 {
			continue
		}
		value, err := decodeJSONValue(string(raw))
		if err != nil {
			parseError = true
			continue
		}
		lines = append(lines, jsonLine{Value: value, Sequence: sequence})
	}
	return lines, parseError, truncated, false
}

// readArtifactJSON reads a single JSON value within the same per-artifact
// budget. Oversized JSON cannot be decoded safely from a fragment.
func readArtifactJSON(path string) (value any, parseError bool, truncated bool, unreadable bool) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false, false, true
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, false, false, true
	}
	if info.Size() > DefaultScanFileBytes {
		return nil, false, true, false
	}
	contents, err := io.ReadAll(io.LimitReader(file, DefaultScanFileBytes))
	if err != nil {
		return nil, false, false, true
	}
	if len(bytes.TrimSpace(contents)) == 0 {
		return nil, false, false, false
	}
	value, err = decodeJSONValue(string(contents))
	if err != nil {
		return nil, true, false, false
	}
	return value, false, false, false
}

func parseEventTimestamp(values ...any) (time.Time, bool) {
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			trimmed := strings.TrimSpace(typed)
			if trimmed == "" {
				continue
			}
			for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.999999999Z07:00"} {
				if parsed, err := time.Parse(layout, trimmed); err == nil {
					return parsed, true
				}
			}
			if numeric, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
				if parsed, ok := unixTimestamp(numeric); ok {
					return parsed, true
				}
			}
		case json.Number:
			if numeric, err := typed.Int64(); err == nil {
				if parsed, ok := unixTimestamp(numeric); ok {
					return parsed, true
				}
			}
		case float64:
			if parsed, ok := unixTimestamp(int64(typed)); ok {
				return parsed, true
			}
		case int64:
			if parsed, ok := unixTimestamp(typed); ok {
				return parsed, true
			}
		case int:
			if parsed, ok := unixTimestamp(int64(typed)); ok {
				return parsed, true
			}
		}
	}
	return time.Time{}, false
}

func unixTimestamp(value int64) (time.Time, bool) {
	if value <= 0 {
		return time.Time{}, false
	}
	switch {
	case value >= 1_000_000_000_000_000:
		return time.UnixMicro(value), true
	case value >= 1_000_000_000_000:
		return time.UnixMilli(value), true
	default:
		return time.Unix(value, 0), true
	}
}

func isRequestedDay(timestamp time.Time, day time.Time) bool {
	return !timestamp.IsZero() && sameLocalDate(timestamp, day)
}
