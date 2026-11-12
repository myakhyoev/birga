package rest

import (
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

const (
	mediaFormField = "file"
	kindFormField  = "kind"
	cropFormField  = "crop"
	// mediaVariantHeader on GET /v1/media/{id} says which size the redirect opens: the asked size, or
	// "original" while the variants are not made yet. Clients that keep photos on disk only keep a
	// size once this matches what they asked for.
	mediaVariantHeader = "X-Media-Variant"
	// multipartOverhead is room for multipart boundaries, headers and other form fields.
	multipartOverhead = 64 << 10
)

type mediaView struct {
	ID          string `json:"id" example:"3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f"`
	URL         string `json:"url" example:"/v1/media/3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f"`
	ContentType string `json:"content_type" example:"image/jpeg"`
	Size        int64  `json:"size" example:"183244"` // bytes
	// Kind is profile or content (the variant sizes made); empty for a video.
	Kind string `json:"kind" example:"content"`
	// VariantsStatus is pending right after an upload (sizes are being made), done, failed, or none (videos).
	VariantsStatus string `json:"variants_status" example:"pending"`
}

func newMediaView(m domain.Media) mediaView {
	return mediaView{
		ID: m.ID, URL: mediaPath(m.ID), ContentType: m.ContentType, Size: m.Size,
		Kind: string(m.Kind), VariantsStatus: m.VariantsStatus,
	}
}

// UploadMedia godoc swagger
// @Summary uploads an image (a profile photo) to S3
// @Description - send form data (multipart/form-data or application/x-www-form-urlencoded) with one field "file":
// @Description   either the file itself (multipart file part) or the file as a base64 string; a data URL
// @Description   prefix such as "data:image/png;base64," is allowed
// @Description - accepted: JPEG, PNG or WebP, detected from the bytes, not from the file name (422 otherwise)
// @Description - at most MEDIA_MAX_SIZE bytes after decoding, default 100 MiB (422 otherwise)
// @Description - optional field "kind": profile (default here) or content; it decides the sizes the
// @Description   server makes in the background (see GET /v1/media/{id}?size=)
// @Description - optional field "crop": "x,y,w,h", the frame the user chose, in fractions (0-1) of the
// @Description   upright photo's width and height; without it the centre is used
// @Description - the answer's id goes into users.photo_id (PATCH /v1/admin/users/{id} {"photo_id": id});
// @Description   url is where clients load the image from: GET /v1/media/{id} on this API
// @Description - 503 when S3 is not configured (S3_BUCKET empty)
// @Tags media
// @Accept multipart/form-data
// @Accept x-www-form-urlencoded
// @Produce json
// @Param file formData file true "the image: a file part, or a base64 string in the same field"
// @Param kind formData string false "profile (default) or content" Enums(profile, content)
// @Param crop formData string false "chosen frame x,y,w,h in fractions of the photo, e.g. 0,0.1,1,0.8"
// @Success 200 {object} rest.R{data=rest.mediaView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Failure 503 {object} rest.ServiceUnavailableResponse
// @Router /v1/media [POST]
func (s *Server) UploadMedia() gin.HandlerFunc {
	return s.uploadMedia(false, domain.MediaKindProfile)
}

// AdminUploadMedia godoc swagger
// @Summary uploads an image or a video (a daily video) to S3
// @Description - same form and limits as POST /v1/media; also accepts MP4 and WebM videos, detected from the bytes
// @Description - the answer's id goes into posts.video_id; GET /v1/media/{id} redirects to the file, video players follow it
// @Description - "kind" defaults to content here (activity and article pictures, video covers); send
// @Description   kind=profile for a user's photo. Videos get no sizes, kind and crop are ignored for them
// @Tags admin
// @Security AdminKey
// @Accept multipart/form-data
// @Accept x-www-form-urlencoded
// @Produce json
// @Param file formData file true "the image or video: a file part, or a base64 string in the same field"
// @Param kind formData string false "content (default) or profile" Enums(profile, content)
// @Param crop formData string false "chosen frame x,y,w,h in fractions of the photo, e.g. 0,0.1,1,0.8"
// @Success 200 {object} rest.R{data=rest.mediaView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Failure 503 {object} rest.ServiceUnavailableResponse
// @Router /v1/admin/media [POST]
func (s *Server) AdminUploadMedia() gin.HandlerFunc {
	return s.uploadMedia(true, domain.MediaKindContent)
}

func (s *Server) uploadMedia(allowVideo bool, defaultKind domain.MediaKind) gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.mediaUploader == nil {
			fail(c, http.StatusServiceUnavailable, _errCodeUnavailable, "media uploads are disabled")

			return
		}

		maxSize := s.mediaUploader.MaxSize()
		// base64 is 4/3 the size of the file it encodes.
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSize/3*4+4+multipartOverhead)

		data, err := readMediaField(c, maxSize)
		if err != nil {
			Return(c, nil, err)

			return
		}

		kind, crop, err := readKindCrop(c, defaultKind)
		if err != nil {
			Return(c, nil, err)

			return
		}

		m, err := s.mediaUploader.Execute(c.Request.Context(), domain.MediaUpload{Data: data, AllowVideo: allowVideo, Kind: kind, Crop: crop})
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, newMediaView(m), nil)
	}
}

// readMediaField returns the bytes of the "file" form field: a multipart file part, or a base64 string.
// It reads at most maxSize+1 bytes of a file part so the use case can reject a file that is too big.
func readMediaField(c *gin.Context, maxSize int64) ([]byte, error) {
	// Parse the form explicitly: c.FormFile hides the error of a urlencoded body (e.g. too large).
	var err error
	if c.ContentType() == gin.MIMEMultipartPOSTForm {
		err = c.Request.ParseMultipartForm(maxSize + multipartOverhead)
	} else {
		err = c.Request.ParseForm()
	}

	if err != nil {
		return nil, formError(err, maxSize)
	}

	if c.Request.MultipartForm != nil && len(c.Request.MultipartForm.File[mediaFormField]) > 0 {
		f, err := c.Request.MultipartForm.File[mediaFormField][0].Open()
		if err != nil {
			return nil, errs.Wrap(err)
		}
		defer f.Close()

		data, err := io.ReadAll(io.LimitReader(f, maxSize+1))
		if err != nil {
			return nil, errs.Wrap(err)
		}

		return data, nil
	}

	value, ok := c.GetPostForm(mediaFormField)
	if !ok || strings.TrimSpace(value) == "" {
		return nil, errs.Errf(errs.ErrBadRequest, "form field %q is missing: send the file or its base64 string", mediaFormField)
	}

	data, err := decodeBase64(value)
	if err != nil {
		return nil, errs.Errf(errs.ErrBadRequest, "form field %q is not valid base64", mediaFormField)
	}

	return data, nil
}

// readKindCrop reads the optional "kind" and "crop" form fields (the form is parsed already).
func readKindCrop(c *gin.Context, defaultKind domain.MediaKind) (domain.MediaKind, *domain.MediaCrop, error) {
	kind := defaultKind

	if v := strings.TrimSpace(c.PostForm(kindFormField)); v != "" {
		k, ok := domain.ParseMediaKind(v)
		if !ok {
			return "", nil, errs.Errf(errs.ErrValidation, "kind must be profile or content")
		}

		kind = k
	}

	v := strings.TrimSpace(c.PostForm(cropFormField))
	if v == "" {
		return kind, nil, nil
	}

	crop, err := parseCrop(v)
	if err != nil {
		return "", nil, err
	}

	return kind, crop, nil
}

// parseCrop reads "x,y,w,h" fractions.
func parseCrop(v string) (*domain.MediaCrop, error) {
	parts := strings.Split(v, ",")
	if len(parts) != 4 { //nolint:mnd
		return nil, errs.Errf(errs.ErrValidation, "crop must be x,y,w,h fractions, e.g. 0,0.1,1,0.8")
	}

	var n [4]float64

	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return nil, errs.Errf(errs.ErrValidation, "crop must be x,y,w,h fractions, e.g. 0,0.1,1,0.8")
		}

		n[i] = f
	}

	crop := &domain.MediaCrop{X: n[0], Y: n[1], W: n[2], H: n[3]}
	if !crop.Valid() {
		return nil, errs.Errf(errs.ErrValidation, "crop must lie inside the photo: x, y >= 0, w, h > 0, x+w <= 1, y+h <= 1")
	}

	return crop, nil
}

// RecropMediaRequest is the new frame for a photo.
type RecropMediaRequest struct {
	// Kind is profile or content.
	Kind string `json:"kind" example:"content"`
	// Crop is the frame in fractions of the upright photo; null means the centre.
	Crop *domain.MediaCrop `json:"crop"`
}

// AdminRecropMedia godoc swagger
// @Summary makes a copy of a photo with another frame
// @Description - takes the original of photo {id} and stores it again as a new media row with this kind
// @Description   and frame; the sizes are made in the background like after an upload
// @Description - media ids never change their picture (apps keep photos by id), so put the new id into
// @Description   the activity, article, video cover or user (photo_id) to show the new frame
// @Description - crop is {"x","y","w","h"} in fractions (0-1) of the upright photo; null means the centre
// @Description - a video: 422; unknown id: 404
// @Tags admin
// @Security AdminKey
// @Accept json
// @Produce json
// @Param id path string true "media id of the photo"
// @Param body body rest.RecropMediaRequest true "kind and frame"
// @Success 200 {object} rest.R{data=rest.mediaView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Failure 503 {object} rest.ServiceUnavailableResponse
// @Router /v1/admin/media/{id}/recrop [POST]
func (s *Server) AdminRecropMedia() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.mediaUploader == nil {
			fail(c, http.StatusServiceUnavailable, _errCodeUnavailable, "media uploads are disabled")

			return
		}

		var req RecropMediaRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Return(c, nil, errs.Errf(errs.ErrBadRequest, "invalid JSON body: %s", err.Error()))

			return
		}

		kind, ok := domain.ParseMediaKind(req.Kind)
		if !ok {
			Return(c, nil, errs.Errf(errs.ErrValidation, "kind must be profile or content"))

			return
		}

		m, err := s.mediaUploader.Recrop(c.Request.Context(), c.Param("id"), kind, req.Crop)
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, newMediaView(m), nil)
	}
}

func formError(err error, maxSize int64) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return errs.Errf(errs.ErrValidation, "request is too large, the file limit is %d bytes", maxSize)
	}

	return errs.Errf(errs.ErrBadRequest, "invalid form data: %s", err.Error())
}

// decodeBase64 accepts standard or URL-safe base64, padded or not, with an optional data URL prefix
// ("data:image/png;base64,") and line breaks.
func decodeBase64(s string) ([]byte, error) {
	if strings.HasPrefix(s, "data:") {
		if i := strings.Index(s, ","); i >= 0 {
			s = s[i+1:]
		}
	}

	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}

		return r
	}, s)

	// In x-www-form-urlencoded bodies an unescaped '+' arrives as ' ' and is dropped above;
	// clients must URL-encode the value (every HTTP client does for form data).
	var err error

	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		var data []byte
		if data, err = enc.DecodeString(s); err == nil {
			return data, nil
		}
	}

	return nil, err
}

// mediaPath is the API path that serves the media with this id (GET /v1/media/{id}).
func mediaPath(id string) string {
	return "/v1/media/" + id
}

// photoURL is mediaPath for a photo id, or nil without a photo. It opens the original.
func photoURL(id *string) *string {
	if id == nil || *id == "" {
		return nil
	}

	p := mediaPath(*id)

	return &p
}

// photoSizeURL is the path of one size of a photo (GET /v1/media/{id}?size=), or nil without a photo.
func photoSizeURL(id *string, size string) *string {
	if id == nil || *id == "" {
		return nil
	}

	p := mediaPath(*id) + "?size=" + size

	return &p
}

// photoURLs are the links to every size of a photo, put next to photo_url in the API's views.
type photoURLs struct {
	// PhotoMainURL is the large size (content 1200x600, profile 800x800); null without a photo.
	PhotoMainURL *string `json:"photo_main_url" example:"/v1/media/5d2f8a90-1c3b-4e6f-8a7d-9b0c1d2e3f4a?size=main"`
	// PhotoThumbURL is the card size (content 400x200, profile 250x250); null without a photo.
	PhotoThumbURL *string `json:"photo_thumb_url" example:"/v1/media/5d2f8a90-1c3b-4e6f-8a7d-9b0c1d2e3f4a?size=thumb"`
	// PhotoSmallURL is the small square for list rows (content 240x240; a profile photo gives its
	// 250x250 thumb); null without a photo.
	PhotoSmallURL *string `json:"photo_small_url" example:"/v1/media/5d2f8a90-1c3b-4e6f-8a7d-9b0c1d2e3f4a?size=small"`
}

func newPhotoURLs(id *string) photoURLs {
	return photoURLs{
		PhotoMainURL:  photoSizeURL(id, domain.MediaSizeMain),
		PhotoThumbURL: photoSizeURL(id, domain.MediaSizeThumb),
		PhotoSmallURL: photoSizeURL(id, domain.MediaSizeSmall),
	}
}

// GetMedia godoc swagger
// @Summary opens an uploaded image (or one of its sizes)
// @Description - redirects (302) to a short-lived signed link to the image in the private S3 bucket
// @Description   (valid MEDIA_LINK_TTL, default 1 hour); image widgets follow it on their own
// @Description - size: original (default), main, thumb or small. Content photos (activities, articles,
// @Description   video covers): main 1200x600, thumb 400x200, small 240x240 (centre square). Profile photos:
// @Description   main 800x800, thumb 250x250 (small gives thumb). Sizes are made in the background a few
// @Description   seconds after the upload; until then, and for videos, the link opens the original
// @Description - header X-Media-Variant says which one the redirect opens (the size, or "original")
// @Description - needs no token, so <img> and Image.network work; ids are random UUIDs
// @Description - unknown id: 404; S3 not configured: 503
// @Tags media
// @Param id path string true "media id (photo_id)"
// @Param size query string false "original (default), main, thumb or small" Enums(original, main, thumb, small)
// @Success 302
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Failure 503 {object} rest.ServiceUnavailableResponse
// @Router /v1/media/{id} [GET]
func (s *Server) GetMedia() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.mediaLinker == nil {
			fail(c, http.StatusServiceUnavailable, _errCodeUnavailable, "media is disabled")

			return
		}

		size := c.DefaultQuery("size", domain.MediaSizeOriginal)
		if !domain.IsMediaSize(size) {
			Return(c, nil, errs.Errf(errs.ErrBadRequest, "size must be original, main, thumb or small"))

			return
		}

		link, served, err := s.mediaLinker.Link(c.Request.Context(), c.Param("id"), size)
		if err != nil {
			Return(c, nil, err)

			return
		}

		c.Header(mediaVariantHeader, served)

		// The signed link expires, so clients must not cache the redirect longer than it lives.
		c.Header("Cache-Control", "private, max-age=300")
		c.Redirect(http.StatusFound, link)
	}
}
