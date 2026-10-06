package domain

import (
	"time"
	"unicode/utf8"
)

// MaxGoalNameLength caps each goal name, counted in characters.
const MaxGoalNameLength = 100

// MaxUserGoals caps how many goals a user can pick.
const MaxUserGoals = 20

// Goal is a development goal a user can pick at sign-up, named in Uzbek, Russian and English.
// Not to be confused with Activity.Goal, the fixed goal key of an activity.
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
