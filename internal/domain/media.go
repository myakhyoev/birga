package domain

import "time"

// Media is an uploaded file stored in object storage (S3). URL is built from Key at read time.
type Media struct {
	ID          string
	Key         string
	URL         string
	ContentType string
	Size        int64
	CreatedAt   time.Time
}

// MediaUpload is a file to store, already decoded from the request.
type MediaUpload struct {
	Data []byte
}

// imageExtensions maps the accepted image types, as sniffed from the content, to file extensions.
var imageExtensions = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

// ImageExtension returns the file extension for an accepted image content type.
func ImageExtension(contentType string) (string, bool) {
	ext, ok := imageExtensions[contentType]

	return ext, ok
}
