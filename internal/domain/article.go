package domain

import "time"

// Article publication states, stored as text and checked by articles_status_chk. Only published
// articles reach the app.
const (
	ArticleStatusPublished   = "published"
	ArticleStatusInReview    = "in_review"
	ArticleStatusUnpublished = "unpublished"
)

// MaxArticleReadMinutes is the longest expected_read_time the application accepts.
const MaxArticleReadMinutes = 60

// MaxArticleSearchLength caps the title search text, counted in characters.
const MaxArticleSearchLength = 100

// IsKnownArticleStatus reports whether status is one of the stored values.
func IsKnownArticleStatus(status string) bool {
	return status == ArticleStatusPublished || status == ArticleStatusInReview || status == ArticleStatusUnpublished
}

// Article is a short text for parents. It serves one goal and an age range.
type Article struct {
	ID          string
	TitleUz     string
	TitleRu     string
	TitleEn     string
	BodyUz      string
	BodyRu      string
	BodyEn      string
	TipUz       string // the optional "try it today" box; "" means none
	TipRu       string
	TipEn       string
	PhotoID     *string
	ReadMinutes int // expected_read_time
	MinAge      int
	MaxAge      int
	GoalID      string
	// TopicID is the optional topic tag; nil means none.
	TopicID *string
	Status  string
	// Source is where the article comes from: SourceTest, SourceAdmin, SourceHarvard or SourceAI.
	Source    string
	CreatedAt time.Time
	UpdatedAt time.Time
	// IsRead tells whether the signed-in user marked the article read. It is only filled on
	// user-scoped reads.
	IsRead bool
	// Locked is set on app lists for a free-plan user: the article was never their daily article,
	// so it does not open (GET /v1/articles/{id} is 403).
	Locked bool
}

// ArticleUpdate is a partial update. A nil field is left unchanged; PhotoID pointing to "" clears
// the photo, and a tip pointing to "" removes it.
type ArticleUpdate struct {
	TitleUz     *string
	TitleRu     *string
	TitleEn     *string
	BodyUz      *string
	BodyRu      *string
	BodyEn      *string
	TipUz       *string
	TipRu       *string
	TipEn       *string
	PhotoID     *string
	ReadMinutes *int
	MinAge      *int
	MaxAge      *int
	GoalID      *string
	// TopicID pointing to "" removes the topic.
	TopicID *string
	Status  *string
	Source  *string
}

// IsEmpty reports whether the update changes nothing.
func (u ArticleUpdate) IsEmpty() bool {
	return u.TitleUz == nil && u.TitleRu == nil && u.TitleEn == nil && u.BodyUz == nil && u.BodyRu == nil &&
		u.BodyEn == nil && u.TipUz == nil && u.TipRu == nil && u.TipEn == nil && u.PhotoID == nil &&
		u.ReadMinutes == nil && u.MinAge == nil && u.MaxAge == nil && u.GoalID == nil && u.TopicID == nil && u.Status == nil && u.Source == nil
}

// ArticleFilter narrows an article listing. Zero values mean "no filter".
type ArticleFilter struct {
	Age     int
	GoalID  string
	TopicID string
	// Search matches articles whose title in any language contains it, ignoring case.
	Search string
	// Status is one of the article statuses; empty lists all.
	Status string
	// Source keeps content of this source; empty lists all.
	Source string
	// MaxMinutes keeps articles of at most this expected read time.
	MaxMinutes int
	// LockFor, when set, marks the articles that were never this user's daily article as Locked.
	LockFor string
	// ReaderID, when set, fills Article.IsRead for that user.
	ReaderID string
	Limit    int
	Offset   int
}

// DailyArticle is the article a user gets for one local day.
type DailyArticle struct {
	Day     time.Time // a date: midnight UTC of the local day
	Article Article
}

// DailyArticles is the answer of the daily article endpoint: today's article and, when one is
// available, the one prepared for tomorrow.
type DailyArticles struct {
	Today    DailyArticle
	Tomorrow *Article
}
