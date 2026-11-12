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
	TitleEn         string
	DescriptionUz   string
	DescriptionRu   string
	DescriptionEn   string
	MainIdeaUz      string // optional "Katta g'oya" text; empty means the app hides the box
	MainIdeaRu      string
	MainIdeaEn      string
	EasierUz        string // optional "Osonroq" variant (Pro); empty means none
	EasierRu        string
	EasierEn        string
	HarderUz        string // optional "Qiyinroq" variant (Pro); empty means none
	HarderRu        string
	HarderEn        string
	Difficulty      string   // DifficultyEasy, DifficultyNormal or DifficultyDifficult
	Source          string   // where the activity comes from: SourceTest, SourceAdmin, SourceHarvard or SourceAI
	PhotoID         *string  // picture of the activity, a media id; nil means none
	GoalIDs         []string // ids of the goals table rows the activity serves, at least one
	MinAge          int
	MaxAge          int
	DurationMinutes int
	IsPublished     bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Activity difficulty levels ("Osonroq", "Odatdagi", "Qiyinroq"). Plans and picks prefer normal ones.
const (
	DifficultyEasy      = "easy"
	DifficultyNormal    = "normal"
	DifficultyDifficult = "difficult"
)

// ValidDifficulty reports whether d is one of the difficulty levels.
func ValidDifficulty(d string) bool {
	return d == DifficultyEasy || d == DifficultyNormal || d == DifficultyDifficult
}

// Activity publication states an admin listing can filter on.
const (
	ActivityStatusPublished = "published"
	ActivityStatusDraft     = "draft"
)

// MaxActivitySearchLength caps the title search text, counted in characters.
const MaxActivitySearchLength = 100

// ActivityFilter narrows an activity listing. Zero values mean "no filter".
type ActivityFilter struct {
	Age    int
	GoalID string
	// Search matches activities whose Uzbek or Russian title contains it, ignoring case.
	Search string
	// Status is ActivityStatusPublished or ActivityStatusDraft; empty lists both.
	Status        string
	PublishedOnly bool
	// MaxMinutes keeps activities of at most this many minutes ("10 daqiqagacha").
	MaxMinutes int
	// Difficulty keeps activities of this level.
	Difficulty string
	// Source keeps activities of this content source.
	Source string
	Limit  int
	Offset int
}

// MaxActivityGoals caps how many goals one activity can serve.
const MaxActivityGoals = 5

// MaxActivityDurationMinutes is the longest activity the application accepts.
const MaxActivityDurationMinutes = 60

// ActivityUpdate is a partial update. A nil field is left unchanged.
type ActivityUpdate struct {
	TitleUz         *string
	TitleRu         *string
	TitleEn         *string
	DescriptionUz   *string
	DescriptionRu   *string
	DescriptionEn   *string
	MainIdeaUz      *string
	MainIdeaRu      *string
	MainIdeaEn      *string
	EasierUz        *string
	EasierRu        *string
	EasierEn        *string
	HarderUz        *string
	HarderRu        *string
	HarderEn        *string
	Difficulty      *string
	Source          *string
	PhotoID         *string  // "" removes the picture
	GoalIDs         []string // nil keeps the goals; a non-nil list replaces them
	MinAge          *int
	MaxAge          *int
	DurationMinutes *int
	IsPublished     *bool
}

// IsEmpty reports whether the update changes nothing.
func (u ActivityUpdate) IsEmpty() bool {
	return u.TitleUz == nil && u.TitleRu == nil && u.TitleEn == nil && u.DescriptionUz == nil && u.DescriptionRu == nil &&
		u.DescriptionEn == nil && u.MainIdeaUz == nil && u.MainIdeaRu == nil && u.MainIdeaEn == nil &&
		u.EasierUz == nil && u.EasierRu == nil && u.EasierEn == nil && u.HarderUz == nil && u.HarderRu == nil && u.HarderEn == nil &&
		u.Difficulty == nil && u.Source == nil && u.PhotoID == nil &&
		u.GoalIDs == nil && u.MinAge == nil && u.MaxAge == nil && u.DurationMinutes == nil && u.IsPublished == nil
}
