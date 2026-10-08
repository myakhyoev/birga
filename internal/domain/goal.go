package domain

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// MaxGoalNameLength caps each goal name, counted in characters.
const MaxGoalNameLength = 100

// MaxUserGoals caps how many goals a user can pick.
const MaxUserGoals = 20

// Goal is a development goal a user can pick at sign-up, named in Uzbek, Russian and English.
// Activities point at the goals they serve (Activity.GoalIDs).
type Goal struct {
	ID        string
	NameUz    string
	NameRu    string
	NameEn    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// GoalUpdate is a partial update. A nil field is left unchanged.
type GoalUpdate struct {
	NameUz *string
	NameRu *string
	NameEn *string
}

// IsEmpty reports whether the update changes nothing.
func (u GoalUpdate) IsEmpty() bool {
	return u.NameUz == nil && u.NameRu == nil && u.NameEn == nil
}

// IsValidGoalName reports whether name is non-empty and at most MaxGoalNameLength characters.
func IsValidGoalName(name string) bool {
	return name != "" && utf8.RuneCountInString(name) <= MaxGoalNameLength
}

// NormalizeIDs writes valid UUIDs in canonical form and drops repeats, keeping the first occurrence's
// order. Invalid ids are kept as they are for the caller's validation to report.
func NormalizeIDs(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))

	for _, id := range ids {
		if u, err := uuid.Parse(strings.TrimSpace(id)); err == nil {
			id = u.String()
		}

		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}

	return out
}
