package memory

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/karanagi/loom/internal/store"
)

type Entry struct {
	ID        string    `yaml:"id"`
	Title     string    `yaml:"title"`
	Type      string    `yaml:"type"`
	Timestamp time.Time `yaml:"timestamp"`
	// Decision
	DecidedBy    string        `yaml:"decided_by,omitempty"`
	Context      string        `yaml:"context,omitempty"`
	Decision     string        `yaml:"decision,omitempty"`
	Rationale    string        `yaml:"rationale,omitempty"`
	Alternatives []Alternative `yaml:"alternatives,omitempty"`
	// Discovery
	DiscoveredBy string `yaml:"discovered_by,omitempty"`
	Location     string `yaml:"location,omitempty"`
	Finding      string `yaml:"finding,omitempty"`
	Implications string `yaml:"implications,omitempty"`
	// Convention
	EstablishedBy string   `yaml:"established_by,omitempty"`
	Rule          string   `yaml:"rule,omitempty"`
	Examples      []string `yaml:"examples,omitempty"`
	AppliesTo     string   `yaml:"applies_to,omitempty"`
	// Common
	Affects []string `yaml:"affects,omitempty"`
	Tags    []string `yaml:"tags,omitempty"`
}

type Alternative struct {
	Option          string `yaml:"option"`
	RejectedBecause string `yaml:"rejected_because"`
}

type AddOpts struct {
	Type      string
	Title     string
	Rationale string
	// Decision fields
	Decision  string
	Context   string
	// Discovery fields
	Finding      string
	Location     string
	Implications string
	// Convention fields
	Rule      string
	AppliesTo string
	// Common
	By      string
	Affects []string
	Tags    []string
}

type SearchOpts struct {
	Query string
	Limit int
}

type ListOpts struct {
	Type    string
	Affects string
}

type SearchResult struct {
	Entry *Entry
	Score float64
}

var typeInfo = map[string]struct {
	prefix string
	subdir string
}{
	"decision":   {"DEC", "decisions"},
	"discovery":  {"DISC", "discoveries"},
	"convention": {"CONV", "conventions"},
}

func prefixToType(prefix string) (string, string) {
	for _, ti := range typeInfo {
		if ti.prefix == prefix {
			return ti.prefix, ti.subdir
		}
	}
	return "", ""
}

func Add(loomRoot string, entry *Entry) error {
	ti, ok := typeInfo[entry.Type]
	if !ok {
		return fmt.Errorf("unknown memory type: %s", entry.Type)
	}
	dir := filepath.Join(loomRoot, "memory", ti.subdir)
	n, err := store.NextCounter(filepath.Join(dir, "counter.txt"))
	if err != nil {
		return err
	}
	entry.ID = fmt.Sprintf("%s-%03d", ti.prefix, n)
	entry.Timestamp = time.Now()
	return store.WriteYAML(filepath.Join(dir, entry.ID+".yaml"), entry)
}

func Load(loomRoot string, id string) (*Entry, error) {
	parts := strings.SplitN(id, "-", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid memory ID: %s", id)
	}
	prefix, subdir := prefixToType(parts[0])
	if prefix == "" {
		return nil, fmt.Errorf("unknown memory ID prefix: %s", parts[0])
	}
	entry := &Entry{}
	if err := store.ReadYAML(filepath.Join(loomRoot, "memory", subdir, id+".yaml"), entry); err != nil {
		return nil, err
	}
	return entry, nil
}

func List(loomRoot string, opts ListOpts) ([]*Entry, error) {
	var entries []*Entry
	for typ, ti := range typeInfo {
		if opts.Type != "" && opts.Type != typ {
			continue
		}
		dir := filepath.Join(loomRoot, "memory", ti.subdir)
		files, err := store.ListYAMLFiles(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			var e Entry
			if err := store.ReadYAML(f, &e); err != nil {
				continue
			}
			if opts.Affects != "" && !contains(e.Affects, opts.Affects) {
				continue
			}
			entries = append(entries, &e)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})
	return entries, nil
}

func Search(loomRoot string, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 5
	}
	terms := tokenize(query)
	if len(terms) == 0 {
		return nil, nil
	}
	all, err := List(loomRoot, ListOpts{})
	if err != nil {
		return nil, err
	}
	var results []SearchResult
	for _, e := range all {
		blob := tokenize(strings.Join([]string{e.Title, e.Context, e.Decision, e.Rationale, e.Finding, e.Rule, e.Implications}, " "))
		if len(blob) == 0 {
			continue
		}
		freq := make(map[string]int)
		for _, w := range blob {
			freq[w]++
		}
		var score float64
		for _, t := range terms {
			if c, ok := freq[t]; ok {
				score += float64(c) / float64(len(blob))
			}
		}
		if score > 0 {
			results = append(results, SearchResult{Entry: e, Score: math.Round(score*100) / 100})
		}
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func tokenize(s string) []string {
	var tokens []string
	for _, w := range strings.Fields(strings.ToLower(s)) {
		w = strings.Trim(w, ".,;:!?\"'()[]{}|")
		if w != "" {
			tokens = append(tokens, w)
		}
	}
	return tokens
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

// Snippet returns the most relevant field content for display.
func Snippet(e *Entry) string {
	switch e.Type {
	case "decision":
		if e.Decision != "" {
			return truncate(e.Decision, 80)
		}
		return truncate(e.Rationale, 80)
	case "discovery":
		return truncate(e.Finding, 80)
	case "convention":
		return truncate(e.Rule, 80)
	}
	return ""
}

// ByField returns the "by" field for the entry type.
func ByField(e *Entry) string {
	switch e.Type {
	case "decision":
		return e.DecidedBy
	case "discovery":
		return e.DiscoveredBy
	case "convention":
		return e.EstablishedBy
	}
	return ""
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n-3] + "..."
	}
	return s
}
