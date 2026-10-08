package domain

import "time"

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
	GoalIDs         []string // ids of the goals table rows the activity serves, at least one
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
	GoalID        string
	PublishedOnly bool
	Limit         int
	Offset        int
}

// MaxActivityGoals caps how many goals one activity can serve.
const MaxActivityGoals = 5

// MaxActivityDurationMinutes is the longest activity the application accepts.
const MaxActivityDurationMinutes = 60

// ActivityUpdate is a partial update. A nil field is left unchanged.
type ActivityUpdate struct {
	TitleUz         *string
	TitleRu         *string
	DescriptionUz   *string
	DescriptionRu   *string
	GoalIDs         []string // nil keeps the goals; a non-nil list replaces them
	MinAge          *int
	MaxAge          *int
	DurationMinutes *int
	IsPublished     *bool
}

// IsEmpty reports whether the update changes nothing.
func (u ActivityUpdate) IsEmpty() bool {
	return u.TitleUz == nil && u.TitleRu == nil && u.DescriptionUz == nil && u.DescriptionRu == nil &&
		u.GoalIDs == nil && u.MinAge == nil && u.MaxAge == nil && u.DurationMinutes == nil && u.IsPublished == nil
}

// RecommendationQuery describes what a caregiver is looking for today. Zero GoalID or MaxMinutes means
// "any". Activities serving one of PreferGoalIDs come first. Day seeds the tie-break so the suggestion
// stays the same for the whole day.
type RecommendationQuery struct {
	ChildID       string
	Age           int
	GoalID        string
	PreferGoalIDs []string
	MaxMinutes    int
	Day           time.Time
}
