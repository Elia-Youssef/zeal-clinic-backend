package server

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"clinic-api/internal/buildmode"
	"clinic-api/internal/database/store"
)

// The normalizer turns a response into text that is the same on every run:
//   - ids made during the run become <id:N>, numbered by first appearance in
//     the golden file; the seeded ids stay as they are
//   - server timestamps become <ts>
//   - dates near the run's clinic-local day become day offsets, <day+N>, so
//     "now"-relative data reads the same on any day; far dates stay as sent
//   - tokens, password hashes, addresses and the build version become
//     placeholders
//   - arrays of objects are sorted by their content, except where a case
//     asserts the server's order

var (
	anyUUIDRe      = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	dateTextRe     = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}(?:T\d{2}:\d{2}(?::\d{2}(?:\.\d+)?)?(?:Z|[+-]\d{2}:\d{2})?)?`)
	serverStampRe  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`)
	jwtTextRe      = regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`)
	passwordHashRe = regexp.MustCompile(`\$argon2id\$[^"\s]+`)
	pdfStampRe     = regexp.MustCompile(`-\d{13}-[0-9a-f]{6}\.pdf`)
	clockTextRe    = regexp.MustCompile(`Sent at \d{2}:\d{2}:\d{2}`)
	sessionTokenRe = regexp.MustCompile(`"session_token":"[0-9a-f]+"`)
)

// dayWindow is how far from the run's day a date counts as "now"-relative.
// The scenario's fixed dates lie further away than this.
const dayWindow = 120

type idNumbers struct {
	ids  map[string]string
	next int
}

func newIDNumbers() *idNumbers { return &idNumbers{ids: map[string]string{}} }

func (s *idNumbers) placeholder(raw string) string {
	if p, ok := s.ids[raw]; ok {
		return p
	}
	s.next++
	p := fmt.Sprintf("<id:%d>", s.next)
	s.ids[raw] = p
	return p
}

type contractNormalizer struct {
	start   time.Time
	loc     *time.Location // the zone days are counted in
	zone    string         // its name in placeholders
	today   time.Time      // the run's day in loc, at midnight
	fixed   map[string]bool
	version string
	host    string // this machine's address as the server reports it
}

func newContractNormalizer(t *testing.T, start time.Time) *contractNormalizer {
	t.Helper()
	loc := store.ClinicLocation()
	now := start.In(loc)
	return &contractNormalizer{
		start:   start,
		loc:     loc,
		zone:    "clinic",
		today:   time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc),
		fixed:   seededIDs(t),
		version: buildmode.Version,
	}
}

// utc counts days from the run's UTC day, for values that bucket UTC
// timestamps by their UTC date.
func (n *contractNormalizer) utc() *contractNormalizer {
	u := *n
	now := n.start.UTC()
	u.loc, u.zone = time.UTC, "UTC"
	u.today = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return &u
}

// normalize rewrites a decoded JSON value in place and returns it. Arrays at
// the paths in keep (".Data.items" and the like) stay in the server's order.
func (n *contractNormalizer) normalize(ids *idNumbers, v any, keepOrder []string) any {
	v = n.scalars(ids, v, "")
	v = n.sortLists(v, keepOrder, "")
	return n.assign(ids, v)
}

// asSent normalizes a request body without reordering it.
func (n *contractNormalizer) asSent(ids *idNumbers, v any) any {
	return n.assign(ids, n.scalars(ids, v, ""))
}

// text normalizes a single string, numbering new ids at once.
func (n *contractNormalizer) text(ids *idNumbers, s string) string {
	return n.assign(ids, n.str(ids, s, "")).(string)
}

func (n *contractNormalizer) scalars(ids *idNumbers, v any, key string) any {
	switch x := v.(type) {
	case map[string]any:
		for k, c := range x {
			x[k] = n.scalars(ids, c, k)
		}
	case []any:
		for i, c := range x {
			x[i] = n.scalars(ids, c, key)
		}
	case string:
		return n.str(ids, x, key)
	case json.Number:
		if key == "expiresAt" {
			return "<ts>"
		}
	}
	return v
}

// str applies every rule except numbering new ids.
func (n *contractNormalizer) str(ids *idNumbers, s, key string) string {
	switch key {
	case "ipAddress":
		if s != "" {
			return "<ip>"
		}
	case "host":
		if s != "" && s == n.host {
			return "<host>"
		}
	case "version", "current", "latest", "cloud":
		if s == n.version {
			return "<version>"
		}
	case "platform":
		if s == runtime.GOOS {
			return "<os>"
		}
	}
	if n.host != "" {
		s = strings.ReplaceAll(s, "//"+n.host+":", "//<host>:")
	}
	s = jwtTextRe.ReplaceAllString(s, "<jwt>")
	s = passwordHashRe.ReplaceAllString(s, "<hash>")
	s = anyUUIDRe.ReplaceAllStringFunc(s, func(u string) string {
		if n.fixed[u] {
			return u
		}
		if p, ok := ids.ids[u]; ok {
			return p
		}
		return u
	})
	s = dateTextRe.ReplaceAllStringFunc(s, n.date)
	s = pdfStampRe.ReplaceAllString(s, "-<ms>.pdf")
	s = sessionTokenRe.ReplaceAllString(s, `"session_token":"<token>"`)
	return clockTextRe.ReplaceAllString(s, "Sent at <time>")
}

// date rewrites one date or date-time.
func (n *contractNormalizer) date(m string) string {
	loc := n.loc
	day, err := time.ParseInLocation(store.DateFormat, m[:10], loc)
	if err != nil {
		return m
	}
	if len(m) == 10 {
		if off, ok := n.offset(day); ok {
			return off
		}
		return m
	}
	if serverStampRe.MatchString(m) {
		if ts, err := time.Parse(time.RFC3339, m); err == nil &&
			!ts.Before(n.start.Add(-2*time.Second)) && !ts.After(time.Now().Add(2*time.Second)) {
			return "<ts>"
		}
	}
	if ts, err := time.Parse(time.RFC3339Nano, m); err == nil {
		// A clinic-local midnight is written in UTC with the offset of the
		// season; name it by its day instead.
		local := ts.In(loc)
		if local.Hour() == 0 && local.Minute() == 0 && local.Second() == 0 && local.Nanosecond() == 0 {
			if off, ok := n.offset(time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)); ok {
				return off + " 00:00 " + n.zone
			}
		}
	}
	if off, ok := n.offset(day); ok {
		return off + m[10:]
	}
	return m
}

func (n *contractNormalizer) offset(day time.Time) (string, bool) {
	d := int(math.Round(day.Sub(n.today).Hours() / 24))
	if d < -dayWindow || d > dayWindow {
		return "", false
	}
	return fmt.Sprintf("<day%+d>", d), true
}

// sortLists orders every array of objects by its normalized content, except
// the arrays at the paths in keep.
func (n *contractNormalizer) sortLists(v any, keepOrder []string, path string) any {
	switch x := v.(type) {
	case map[string]any:
		for k, c := range x {
			x[k] = n.sortLists(c, keepOrder, path+"."+k)
		}
	case []any:
		for i, c := range x {
			x[i] = n.sortLists(c, keepOrder, path+"[]")
		}
		if slices.Contains(keepOrder, path) {
			return x
		}
		if !slices.ContainsFunc(x, func(c any) bool { _, ok := c.(map[string]any); return !ok }) {
			keys := make([]string, len(x))
			for i, c := range x {
				keys[i] = n.sortKey(c)
			}
			idx := make([]int, len(x))
			for i := range idx {
				idx[i] = i
			}
			sort.SliceStable(idx, func(a, b int) bool { return keys[idx[a]] < keys[idx[b]] })
			sorted := make([]any, len(x))
			for i, j := range idx {
				sorted[i] = x[j]
			}
			return sorted
		}
	}
	return v
}

// sortKey is an element's JSON with the ids not numbered yet masked, so the
// order does not depend on them.
func (n *contractNormalizer) sortKey(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return anyUUIDRe.ReplaceAllStringFunc(string(b), func(u string) string {
		if n.fixed[u] {
			return u
		}
		return "<id:?>"
	})
}

// assign numbers the ids not seen before, in document order: an object's own
// "id" first, then its keys in sorted order.
func (n *contractNormalizer) assign(ids *idNumbers, v any) any {
	switch x := v.(type) {
	case map[string]any:
		if c, ok := x["id"]; ok {
			x["id"] = n.assign(ids, c)
		}
		for _, k := range sortedMapKeys(x) {
			if k != "id" {
				x[k] = n.assign(ids, x[k])
			}
		}
	case []any:
		for i := range x {
			x[i] = n.assign(ids, x[i])
		}
	case string:
		return anyUUIDRe.ReplaceAllStringFunc(x, func(u string) string {
			if n.fixed[u] {
				return u
			}
			return ids.placeholder(u)
		})
	}
	return v
}
