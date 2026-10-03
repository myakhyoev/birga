package domain

import "time"

// Development goals an activity supports. Kept as plain strings so the
// content team can add new ones with a migration, not a code change.
const (
	GoalLanguage  = "language"
	GoalMotor     = "motor"
	GoalCognitive = "cognitive"
	GoalSocial    = "social"
	GoalEmotional = "emotional"
)

// Children the product targets are 2 to 6 years old.
const (
	MinChildAge = 2
	MaxChildAge = 6
)

// Activity is one short, adult-led offline activity.
type Activity struct {
	ID              string
	TitleUz         string
	TitleRu         string
	DescriptionUz   string
	DescriptionRu   string
	Goal            string
	MinAge          int
	MaxAge          int
	DurationMinutes int
	IsPublished     bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ActivityFilter narrows an activity listing. Zero values mean "no filter".
type ActivityFilter struct {
	Age           int
	Goal          string
	PublishedOnly bool
	Limit         int
	Offset        int
}

// IsKnownGoal reports whether goal is one of the defined development goals.
func IsKnownGoal(goal string) bool {
	switch goal {
	case GoalLanguage, GoalMotor, GoalCognitive, GoalSocial, GoalEmotional:
		return true
	default:
		return false
	}
}

// MaxActivityDurationMinutes is the longest activity the application accepts.
const MaxActivityDurationMinutes = 60

// ActivityUpdate is a partial update. A nil field is left unchanged.
type ActivityUpdate struct {
	TitleUz         *string
	TitleRu         *string
	DescriptionUz   *string
	DescriptionRu   *string
	Goal            *string
	MinAge          *int
	MaxAge          *int
	DurationMinutes *int
	IsPublished     *bool
}

// IsEmpty reports whether the update changes nothing.
func (u ActivityUpdate) IsEmpty() bool {
	return u.TitleUz == nil && u.TitleRu == nil && u.DescriptionUz == nil && u.DescriptionRu == nil &&
		u.Goal == nil && u.MinAge == nil && u.MaxAge == nil && u.DurationMinutes == nil && u.IsPublished == nil
}

// RecommendationQuery describes what a caregiver is looking for today. Zero Goal or MaxMinutes means
// "any". Day seeds the tie-break so the suggestion stays the same for the whole day.
type RecommendationQuery struct {
	ChildID    string
	Age        int
	Goal       string
	MaxMinutes int
	Day        time.Time
}
