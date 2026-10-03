package domain

import (
	"time"
	"unicode/utf8"
)

// Child genders, stored as text and checked by children_gender_chk.
const (
	GenderMale   = "male"
	GenderFemale = "female"
)

// MaxChildProfileAge is the oldest age children_age_chk accepts, in years. It is wider than the
// 2 to 6 activity range so a profile stays valid as the child grows.
const MaxChildProfileAge = 18

// MaxChildNameLength caps a child's name, counted in characters.
const MaxChildNameLength = 100

// Child is a child profile. It belongs to one or more parents (users) through user_children.
type Child struct {
	ID        string
	Name      string
	Age       int
	Gender    string
	PhotoID   *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ChildUpdate is a partial update. A nil field is left unchanged; PhotoID pointing to "" clears the photo.
type ChildUpdate struct {
	Name    *string
	Age     *int
	Gender  *string
	PhotoID *string
}

// IsEmpty reports whether the update changes nothing.
func (u ChildUpdate) IsEmpty() bool {
	return u.Name == nil && u.Age == nil && u.Gender == nil && u.PhotoID == nil
}

// ChildFilter narrows a child listing. A non-empty ParentID lists only that user's children.
type ChildFilter struct {
	ParentID string
	Limit    int
	Offset   int
}

// IsKnownGender reports whether gender is one of the stored values.
func IsKnownGender(gender string) bool {
	return gender == GenderMale || gender == GenderFemale
}

// IsValidChildAge reports whether age fits children_age_chk.
func IsValidChildAge(age int) bool {
	return age >= 0 && age <= MaxChildProfileAge
}

// IsValidChildName reports whether name is non-empty and at most MaxChildNameLength characters.
func IsValidChildName(name string) bool {
	return name != "" && utf8.RuneCountInString(name) <= MaxChildNameLength
}
