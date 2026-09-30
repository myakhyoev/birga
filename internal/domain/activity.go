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
