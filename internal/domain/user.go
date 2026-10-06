package domain

import (
	"regexp"
	"time"
	"unicode/utf8"
)

// MaxUserNameLength caps the display name, counted in characters.
const MaxUserNameLength = 100

var (
	// usernameRegex: 3..32 lowercase latin letters, digits, '_' or '.'.
	usernameRegex = regexp.MustCompile(`^[a-z0-9_.]{3,32}$`)
	// phoneRegex: E.164 with a leading '+', at most 15 characters to fit users.phone_number VARCHAR(15).
	phoneRegex = regexp.MustCompile(`^\+[1-9][0-9]{6,13}$`)
)

// Relationships a user can have to the children, the users.relationship enum.
const (
	RelationshipFather   = "father"
	RelationshipMother   = "mother"
	RelationshipEducator = "educator"
	RelationshipNanny    = "nanny"
)

// User is an app user (a parent or caregiver). Nil fields are NULL in the database.
type User struct {
	ID           string
	Name         *string
	Username     *string
	PhoneNumber  *string
	PhotoID      *string
	Relationship *string
	// GoalIDs are the ids of the goals the user picked; never nil when read from the database.
	GoalIDs   []string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// UserUpdate is a partial update. A nil field is left unchanged; a non-nil
// field pointing to "" clears the column (where the column may be cleared).
type UserUpdate struct {
	Name        *string
	Username    *string
	PhoneNumber *string
	PhotoID     *string
}

// IsEmpty reports whether the update changes nothing.
func (u UserUpdate) IsEmpty() bool {
	return u.Name == nil && u.Username == nil && u.PhoneNumber == nil && u.PhotoID == nil
}

// UserFilter narrows a user listing.
type UserFilter struct {
	Limit  int
	Offset int
}

// IsValidUsername reports whether username matches the allowed format.
func IsValidUsername(username string) bool {
	return usernameRegex.MatchString(username)
}

// IsValidPhoneNumber reports whether phone is an E.164 number.
func IsValidPhoneNumber(phone string) bool {
	return phoneRegex.MatchString(phone)
}

// IsKnownRelationship reports whether relationship is one of the users.relationship enum values.
func IsKnownRelationship(relationship string) bool {
	switch relationship {
	case RelationshipFather, RelationshipMother, RelationshipEducator, RelationshipNanny:
		return true
	default:
		return false
	}
}

// IsValidUserName reports whether name fits the length limit.
func IsValidUserName(name string) bool {
	return utf8.RuneCountInString(name) <= MaxUserNameLength
}
