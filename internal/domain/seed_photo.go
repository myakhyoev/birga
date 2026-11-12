package domain

// Kinds of content a seed photo belongs to (seeds/photos/sources.tsv).
const (
	SeedPhotoActivity = "activity" // key: the activity's title_uz
	SeedPhotoArticle  = "article"  // key: the article id
	SeedPhotoPost     = "post"     // key: the video post id; the photo is its cover
)

// SeedPhoto is one starter picture to attach to existing content that has no picture yet.
type SeedPhoto struct {
	Kind string
	Key  string
	File string // path of the JPEG, relative to the seed photo folder
}
