package domain

import "time"

// Post publication states share the article values and are checked by posts_status_chk.
// Only published posts reach the app.

// MaxPostDurationMinutes is the longest video the application accepts, in minutes.
const MaxPostDurationMinutes = 600

// MaxNextPosts is how many upcoming videos the daily video lists (the locked "next videos").
const MaxNextPosts = 2

// Post is a short video for parents and children ("Kun videosi"). It serves one goal and an age range.
type Post struct {
	ID      string
	TitleUz string
	TitleRu string
	TitleEn string
	VideoID string // an uploaded media id holding an MP4 or WebM video
	// PhotoID is the optional cover picture, an uploaded image; nil means none.
	PhotoID *string
	GoalID  string
	// TopicID is the optional topic tag; nil means none.
	TopicID      *string
	MinAge       int
	MaxAge       int
	Duration     float64 // minutes
	SuggestionUz string  // the optional "Tavsiya" line; "" means none
	SuggestionRu string
	SuggestionEn string
	QuestionsUz  string // the optional "ask after watching" box, one question per line; "" means none
	QuestionsRu  string
	QuestionsEn  string
	Status       string
	// Source is where the video comes from: SourceTest, SourceAdmin, SourceHarvard or SourceAI.
	Source string
	// Locked is set on app lists for a free-plan user: the post was never their daily video.
	Locked    bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// PostUpdate is a partial update. A nil field is left unchanged; a suggestion or questions text
// pointing to "" removes it.
type PostUpdate struct {
	TitleUz *string
	TitleRu *string
	TitleEn *string
	VideoID *string
	// PhotoID pointing to "" removes the cover picture.
	PhotoID *string
	GoalID  *string
	// TopicID pointing to "" removes the topic.
	TopicID      *string
	MinAge       *int
	MaxAge       *int
	Duration     *float64
	SuggestionUz *string
	SuggestionRu *string
	SuggestionEn *string
	QuestionsUz  *string
	QuestionsRu  *string
	QuestionsEn  *string
	Status       *string
	Source       *string
}

// IsEmpty reports whether the update changes nothing.
func (u PostUpdate) IsEmpty() bool {
	return u.TitleUz == nil && u.TitleRu == nil && u.TitleEn == nil && u.VideoID == nil && u.PhotoID == nil &&
		u.GoalID == nil && u.TopicID == nil &&
		u.MinAge == nil && u.MaxAge == nil && u.Duration == nil && u.SuggestionUz == nil &&
		u.SuggestionRu == nil && u.SuggestionEn == nil && u.QuestionsUz == nil && u.QuestionsRu == nil &&
		u.QuestionsEn == nil && u.Status == nil && u.Source == nil
}

// PostFilter narrows a post listing. Zero values mean "no filter".
type PostFilter struct {
	Age     int
	GoalID  string
	TopicID string
	// Search matches posts whose title in any language contains it, ignoring case.
	Search string
	// Status is one of the article statuses; empty lists all.
	Status string
	// Source keeps content of this source; empty lists all.
	Source string
	// MaxMinutes keeps videos of at most this many minutes.
	MaxMinutes float64
	// LockFor, when set, marks the posts that were never this user's daily video as Locked.
	LockFor string
	Limit   int
	Offset  int
}

// DailyPost is the video a user gets for one local day.
type DailyPost struct {
	Day  time.Time // a date: midnight UTC of the local day
	Post Post
}

// DailyPosts is the answer of the daily video endpoint: today's video and the next ones, which
// the app shows locked.
type DailyPosts struct {
	Today DailyPost
	Next  []Post
}
