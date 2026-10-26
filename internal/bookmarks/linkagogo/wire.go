package linkagogo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// The API's response shapes are only partly documented, so responses are read
// as loose JSON objects and fields are looked up under every name they might
// have. Folders are kept whole so an update can send back what it did not change.

type obj map[string]json.RawMessage

// decodeObj parses one JSON object.
func decodeObj(data []byte) (obj, error) {
	var o obj
	if err := json.Unmarshal(data, &o); err != nil {
		return nil, fmt.Errorf("unexpected response from LinkaGoGo: %w", err)
	}
	return o, nil
}

// decodeList reads a list that is either a bare array or an object holding
// the array under one of keys.
func decodeList(data []byte, keys ...string) ([]obj, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, nil
	}
	var list []obj
	if data[0] == '[' {
		if err := json.Unmarshal(data, &list); err != nil {
			return nil, fmt.Errorf("unexpected response from LinkaGoGo: %w", err)
		}
		return list, nil
	}
	o, err := decodeObj(data)
	if err != nil {
		return nil, err
	}
	for _, k := range keys {
		if raw, ok := o[k]; ok {
			if err := json.Unmarshal(raw, &list); err != nil {
				return nil, fmt.Errorf("unexpected response from LinkaGoGo: %w", err)
			}
			return list, nil
		}
	}
	return nil, nil
}

func (o obj) raw(names ...string) json.RawMessage {
	for _, n := range names {
		if raw, ok := o[n]; ok && string(raw) != "null" {
			return raw
		}
	}
	return nil
}

// str returns the first of names that holds a string.
func (o obj) str(names ...string) string {
	for _, n := range names {
		var s string
		if raw := o.raw(n); raw != nil && json.Unmarshal(raw, &s) == nil {
			return s
		}
	}
	return ""
}

// id returns an ID that may be a number or a string. 0 and missing are the
// root, which is the empty string everywhere else in the app.
func (o obj) id(names ...string) string {
	raw := o.raw(names...)
	if raw == nil {
		return ""
	}
	s := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if s == "0" {
		return ""
	}
	return s
}

// time returns the first of names that holds a time, as an RFC 3339 string or
// Unix seconds or milliseconds; zero when there is none.
func (o obj) time(names ...string) time.Time {
	for _, n := range names {
		raw := o.raw(n)
		if raw == nil {
			continue
		}
		var s string
		if json.Unmarshal(raw, &s) == nil {
			for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
				if t, err := time.Parse(layout, s); err == nil {
					return t.UTC()
				}
			}
			continue
		}
		var f float64
		if json.Unmarshal(raw, &f) == nil && f > 0 {
			if f > 1e11 { // milliseconds
				return time.UnixMilli(int64(f)).UTC()
			}
			return time.Unix(int64(f), 0).UTC()
		}
	}
	return time.Time{}
}

// jsonID is how an ID goes into a request body: 0 for the root, a number when
// the ID is numeric, otherwise the string itself.
func jsonID(id string) any {
	if id == "" {
		return 0
	}
	if n, err := strconv.ParseInt(id, 10, 64); err == nil {
		return n
	}
	return id
}

// parseKeywords reads keywords sent as a JSON array or as one string, either
// space separated (a keyword with spaces in double quotes) or comma separated.
func parseKeywords(raw json.RawMessage, comma bool) []string {
	out := []string{}
	if raw == nil {
		return out
	}
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		for _, k := range list {
			if k = strings.TrimSpace(k); k != "" {
				out = append(out, k)
			}
		}
		return out
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return out
	}
	if comma {
		for _, k := range strings.Split(s, ",") {
			if k = strings.TrimSpace(k); k != "" {
				out = append(out, k)
			}
		}
		return out
	}
	var cur strings.Builder
	quoted := false
	flush := func() {
		if k := strings.TrimSpace(cur.String()); k != "" {
			out = append(out, k)
		}
		cur.Reset()
	}
	for _, r := range s {
		switch {
		case r == '"':
			quoted = !quoted
		case r == ' ' && !quoted:
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

// formatKeywords is the inverse of parseKeywords for a string body.
func formatKeywords(keywords []string, comma bool) string {
	parts := make([]string, 0, len(keywords))
	for _, k := range keywords {
		k = strings.TrimSpace(strings.ReplaceAll(k, `"`, ""))
		switch {
		case k == "":
		case comma:
			parts = append(parts, strings.ReplaceAll(k, ",", " "))
		case strings.Contains(k, " "):
			parts = append(parts, `"`+k+`"`)
		default:
			parts = append(parts, k)
		}
	}
	if comma {
		return strings.Join(parts, ",")
	}
	return strings.Join(parts, " ")
}
