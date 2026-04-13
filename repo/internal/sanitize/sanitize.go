// Package sanitize provides helpers that clean untrusted strings before they
// are stored in PostgreSQL.  PostgreSQL text and jsonb columns reject the
// Unicode NULL character (U+0000 / \x00), which can appear in user-supplied
// data or in serialised JSON produced by Go's encoding/json package.
package sanitize

import (
	"bytes"
	"encoding/json"
	"strings"
)

// NullChar is the zero byte that PostgreSQL rejects in text columns.
const NullChar = "\x00"

// String removes all null bytes from s.
func String(s string) string {
	return strings.ReplaceAll(s, NullChar, "")
}

// JSON marshals v to JSON and strips any embedded null bytes from the
// resulting byte slice.  This prevents "unsupported Unicode escape sequence"
// errors when the output is stored in a PostgreSQL text or jsonb column.
func JSON(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return bytes.ReplaceAll(b, []byte(NullChar), nil), nil
}

// JSONNullable is like JSON but returns nil when v is nil, matching the
// behaviour expected by nullable database columns.
func JSONNullable(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return JSON(v)
}
