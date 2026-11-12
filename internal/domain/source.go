package domain

import "strings"

// Content sources: where an activity, article or video (post) comes from. Stored as text and
// checked by the <table>_source_chk constraints. Content stored before sources existed is
// SourceTest; new content defaults to SourceAdmin.
const (
	SourceTest    = "test"    // sample content made for testing
	SourceAdmin   = "admin"   // written by an admin in the dashboard
	SourceHarvard = "harvard" // taken from Harvard material
	SourceAI      = "ai"      // generated with AI
)

// ValidSource reports whether s is one of the content sources.
func ValidSource(s string) bool {
	return s == SourceTest || s == SourceAdmin || s == SourceHarvard || s == SourceAI
}

// NormalizeSource trims and lower-cases a source given by a client.
func NormalizeSource(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
