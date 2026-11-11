package rest

import (
	"time"

	"github.com/gin-gonic/gin"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

// CreatePostRequest is the body of POST /v1/admin/posts.
type CreatePostRequest struct {
	TitleUz string `json:"title_uz" example:"Ranglar qo‘shig‘i"`
	TitleRu string `json:"title_ru" example:"Песня о цветах"`
	TitleEn string `json:"title_en" example:"The colours song"`
	VideoID string `json:"video_id" example:"3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f"`
	// PhotoID is the optional cover picture, an image from POST /v1/admin/media; null means none.
	PhotoID *string `json:"photo_id" example:"5d2f8a90-1c3b-4e6f-8a7d-9b0c1d2e3f4a"`
	GoalID  string  `json:"goal_id" example:"2b6f0cc9-0f3e-4b1a-9a7e-5d8c3e2f1a00"`
	// TopicID is the optional topic tag from GET /v1/topics; null means none.
	TopicID *string `json:"topic_id" example:"9a1c7e52-3b4d-4f6a-8e2b-7c5d1f0a9b3e"`
	MinAge  int     `json:"min_age" example:"3" minimum:"2" maximum:"6"`
	MaxAge  int     `json:"max_age" example:"5" minimum:"2" maximum:"6"`
	// Duration is in minutes, a decimal: 4.2 is 4:12.
	Duration     float64 `json:"duration" example:"4.2"`
	SuggestionUz string  `json:"suggestion_uz" example:"2–4 yoshda ekran oldida kuniga 1 soatdan kam vaqt. Videoni birga ko‘ring."`
	SuggestionRu string  `json:"suggestion_ru" example:"В 2–4 года меньше часа у экрана в день. Смотрите видео вместе."`
	SuggestionEn string  `json:"suggestion_en" example:"At 2–4, less than an hour of screen time a day. Watch the video together."`
	QuestionsUz  string  `json:"questions_uz" example:"«Qaysi rang senga eng ko‘p yoqdi?»\n«Uyimizda qizil rangli nima bor?»"`
	QuestionsRu  string  `json:"questions_ru" example:"«Какой цвет тебе понравился больше всего?»\n«Что у нас дома красное?»"`
	QuestionsEn  string  `json:"questions_en" example:"\"Which colour did you like most?\"\n\"What is red in our home?\""`
	Status       string  `json:"status" example:"in_review" enums:"published,in_review,unpublished"`
}

// CreatePost godoc swagger
// @Summary creates a video post ("Kun videosi")
// @Description - title is required in Uzbek, Russian and English; suggestion_* ("Tavsiya" line) and
// @Description   questions_* ("ask after watching" box, one question per line) are optional, "" hides them
// @Description - video_id is an MP4 or WebM video uploaded with POST /v1/admin/media (422 otherwise)
// @Description - photo_id is an optional cover picture, an image uploaded with POST /v1/admin/media (422 if unknown)
// @Description - goal_id is one active goal from GET /v1/goals (422 if unknown)
// @Description - topic_id is an optional topic from GET /v1/topics, shown as the tag instead of the goal (422 if unknown)
// @Description - duration is in minutes, a decimal (4.2 = 4:12), more than 0 and at most 600
// @Description - min_age/max_age are child ages within 2..6
// @Description - status is published, in_review or unpublished; omitted means in_review. Only published posts reach the app
// @Tags admin
// @Security AdminKey
// @Accept json
// @Produce json
// @Param body body CreatePostRequest true "post"
// @Success 200 {object} rest.R{data=rest.postView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/admin/posts [POST]
func (s *Server) CreatePost() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreatePostRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Return(c, nil, errs.Errf(errs.ErrBadRequest, "invalid JSON body: %s", err.Error()))

			return
		}

		p, err := s.postCreator.Execute(c.Request.Context(), domain.Post{
			TitleUz:      req.TitleUz,
			TitleRu:      req.TitleRu,
			TitleEn:      req.TitleEn,
			VideoID:      req.VideoID,
			PhotoID:      req.PhotoID,
			GoalID:       req.GoalID,
			TopicID:      req.TopicID,
			MinAge:       req.MinAge,
			MaxAge:       req.MaxAge,
			Duration:     req.Duration,
			SuggestionUz: req.SuggestionUz,
			SuggestionRu: req.SuggestionRu,
			SuggestionEn: req.SuggestionEn,
			QuestionsUz:  req.QuestionsUz,
			QuestionsRu:  req.QuestionsRu,
			QuestionsEn:  req.QuestionsEn,
			Status:       req.Status,
		})
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toPostView(p), nil)
	}
}

// AdminListPosts godoc swagger
// @Summary lists video posts in every status
// @Tags admin
// @Security AdminKey
// @Produce json
// @Param q query string false "text the title (any language) contains, ignoring case (at most 100 characters)"
// @Param status query string false "published, in_review or unpublished; omitted lists all" Enums(published, in_review, unpublished)
// @Param goal_id query string false "goal id (UUID) from GET /v1/goals"
// @Param topic_id query string false "topic id (UUID) from GET /v1/topics"
// @Param age query int false "child age in years (2..6)"
// @Param max_minutes query int false "at most this many minutes"
// @Param limit query int false "page size (default 20, max 100)"
// @Param offset query int false "offset"
// @Success 200 {object} rest.R{data=rest.postListView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/admin/posts [GET]
func (s *Server) AdminListPosts() gin.HandlerFunc {
	return func(c *gin.Context) {
		f, ok := postFilter(c)
		if !ok {
			return
		}

		f.Status = c.Query("status")

		items, total, err := s.postLister.Execute(c.Request.Context(), f)
		if err != nil {
			Return(c, nil, err)

			return
		}

		views := make([]postView, 0, len(items))
		for i := range items {
			views = append(views, toPostView(items[i]))
		}

		Return(c, postListView{Items: views, Total: total, Limit: f.Limit, Offset: f.Offset}, nil)
	}
}

// AdminGetPost godoc swagger
// @Summary returns a video post in any status
// @Tags admin
// @Security AdminKey
// @Produce json
// @Param id path string true "post id (UUID)"
// @Success 200 {object} rest.R{data=rest.postView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/admin/posts/{id} [GET]
func (s *Server) AdminGetPost() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := ValidateUUID(id); err != nil {
			Return(c, nil, err)

			return
		}

		p, err := s.postGetter.Execute(c.Request.Context(), id)
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toPostView(p), nil)
	}
}

// UpdatePostRequest is the body of PATCH /v1/admin/posts/{id}. Omitted or null fields are left unchanged.
type UpdatePostRequest struct {
	TitleUz *string `json:"title_uz" example:"Ranglar qo‘shig‘i"`
	TitleRu *string `json:"title_ru" example:"Песня о цветах"`
	TitleEn *string `json:"title_en" example:"The colours song"`
	VideoID *string `json:"video_id" example:"3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f"`
	// PhotoID "" removes the cover picture.
	PhotoID *string `json:"photo_id" example:"5d2f8a90-1c3b-4e6f-8a7d-9b0c1d2e3f4a"`
	GoalID  *string `json:"goal_id" example:"2b6f0cc9-0f3e-4b1a-9a7e-5d8c3e2f1a00"`
	// TopicID "" removes the topic; omitted or null keeps it.
	TopicID      *string  `json:"topic_id" example:"9a1c7e52-3b4d-4f6a-8e2b-7c5d1f0a9b3e"`
	MinAge       *int     `json:"min_age" example:"3" minimum:"2" maximum:"6"`
	MaxAge       *int     `json:"max_age" example:"5" minimum:"2" maximum:"6"`
	Duration     *float64 `json:"duration" example:"4.2"`
	SuggestionUz *string  `json:"suggestion_uz"`
	SuggestionRu *string  `json:"suggestion_ru"`
	SuggestionEn *string  `json:"suggestion_en"`
	QuestionsUz  *string  `json:"questions_uz"`
	QuestionsRu  *string  `json:"questions_ru"`
	QuestionsEn  *string  `json:"questions_en"`
	Status       *string  `json:"status" example:"published" enums:"published,in_review,unpublished"`
}

// UpdatePost godoc swagger
// @Summary updates a video post partially, including its status
// @Description - only the fields present in the body change; omitted or null fields are kept
// @Description - same rules as create; titles cannot be emptied; suggestion_*/questions_* "" removes them; photo_id "" removes the cover
// @Description - the resulting age range must stay valid (422)
// @Description - send {"status": "published"} to show it in the app, "in_review" or "unpublished" to hide it
// @Tags admin
// @Security AdminKey
// @Accept json
// @Produce json
// @Param id path string true "post id (UUID)"
// @Param body body UpdatePostRequest true "fields to change"
// @Success 200 {object} rest.R{data=rest.postView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/admin/posts/{id} [PATCH]
func (s *Server) UpdatePost() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := ValidateUUID(id); err != nil {
			Return(c, nil, err)

			return
		}

		var req UpdatePostRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Return(c, nil, errs.Errf(errs.ErrBadRequest, "invalid JSON body: %s", err.Error()))

			return
		}

		p, err := s.postUpdater.Execute(c.Request.Context(), id, domain.PostUpdate{
			TitleUz:      req.TitleUz,
			TitleRu:      req.TitleRu,
			TitleEn:      req.TitleEn,
			VideoID:      req.VideoID,
			PhotoID:      req.PhotoID,
			GoalID:       req.GoalID,
			TopicID:      req.TopicID,
			MinAge:       req.MinAge,
			MaxAge:       req.MaxAge,
			Duration:     req.Duration,
			SuggestionUz: req.SuggestionUz,
			SuggestionRu: req.SuggestionRu,
			SuggestionEn: req.SuggestionEn,
			QuestionsUz:  req.QuestionsUz,
			QuestionsRu:  req.QuestionsRu,
			QuestionsEn:  req.QuestionsEn,
			Status:       req.Status,
		})
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toPostView(p), nil)
	}
}

// DeletePost godoc swagger
// @Summary soft-deletes a video post
// @Description Sets deleted_at; the post disappears from every list, get, daily video and video history.
// @Tags admin
// @Security AdminKey
// @Produce json
// @Param id path string true "post id (UUID)"
// @Success 200 {object} rest.R
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/admin/posts/{id} [DELETE]
func (s *Server) DeletePost() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := ValidateUUID(id); err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, nil, s.postDeleter.Execute(c.Request.Context(), id))
	}
}

// ListPosts godoc swagger
// @Summary lists published video posts as previews, without the video
// @Description Every signed-in user may browse the titles; opening a video follows the rules of GET /v1/posts/{id}.
// @Tags posts
// @Security BearerAuth
// @Produce json
// @Param q query string false "text the title (any language) contains, ignoring case (at most 100 characters)"
// @Param goal_id query string false "goal id (UUID) from GET /v1/goals"
// @Param topic_id query string false "topic id (UUID) from GET /v1/topics"
// @Param age query int false "child age in years (2..6)"
// @Param max_minutes query int false "at most this many minutes"
// @Param limit query int false "page size (default 20, max 100)"
// @Param offset query int false "offset"
// @Success 200 {object} rest.R{data=rest.postTeaserListView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/posts [GET]
func (s *Server) ListPosts() gin.HandlerFunc {
	return func(c *gin.Context) {
		f, ok := postFilter(c)
		if !ok {
			return
		}

		f.Status = domain.ArticleStatusPublished
		if !currentUserPro(c) {
			f.LockFor = currentUserID(c)
		}

		items, total, err := s.postLister.Execute(c.Request.Context(), f)
		if err != nil {
			Return(c, nil, err)

			return
		}

		views := make([]postTeaserView, 0, len(items))
		for i := range items {
			views = append(views, toPostTeaserView(items[i]))
		}

		Return(c, postTeaserListView{Items: views, Total: total, Limit: f.Limit, Offset: f.Offset}, nil)
	}
}

// GetPost godoc swagger
// @Summary returns a published video post with its video
// @Description Users without Birga Pro may open only posts that were their daily video, today or earlier
// @Description (403 otherwise); Pro users may open every published post.
// @Tags posts
// @Security BearerAuth
// @Produce json
// @Param id path string true "post id (UUID)"
// @Success 200 {object} rest.R{data=rest.postView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 403 {object} rest.ForbiddenResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/posts/{id} [GET]
func (s *Server) GetPost() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := ValidateUUID(id); err != nil {
			Return(c, nil, err)

			return
		}

		p, err := s.postGetter.ForUser(c.Request.Context(), currentUserID(c), currentUserPro(c), id)
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toPostView(p), nil)
	}
}

// GetDailyPost godoc swagger
// @Summary returns the signed-in user's video of the day ("Kun videosi")
// @Description The video is picked once per day (Uzbekistan time) and stays the same all day, the same way as the
// @Description daily article: a goal of the user's children first, an age that fits a child, not shown before, then
// @Description rotating through the goals. next lists up to 2 videos that would come next, without the video, for
// @Description the locked "Keyingi videolar" list. 404 when no video is published.
// @Tags posts
// @Security BearerAuth
// @Produce json
// @Success 200 {object} rest.R{data=rest.dailyPostView}
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/me/posts/daily [GET]
func (s *Server) GetDailyPost() gin.HandlerFunc {
	return func(c *gin.Context) {
		d, err := s.dailyPost.Execute(c.Request.Context(), currentUserID(c))
		if err != nil {
			Return(c, nil, err)

			return
		}

		v := dailyPostView{
			Day: d.Today.Day.Format(time.DateOnly), Post: toPostView(d.Today.Post), Next: make([]postTeaserView, 0, len(d.Next)),
		}
		for i := range d.Next {
			next := toPostTeaserView(d.Next[i])
			next.Locked = !currentUserPro(c)
			v.Next = append(v.Next, next)
		}

		Return(c, v, nil)
	}
}

// ListMyPosts godoc swagger
// @Summary lists the signed-in user's daily videos of earlier days and today, newest first
// @Description One item per day the user opened the daily video. Deleted posts are left out.
// @Tags posts
// @Security BearerAuth
// @Produce json
// @Param limit query int false "page size (default 20, max 100)"
// @Param offset query int false "offset"
// @Success 200 {object} rest.R{data=rest.dailyPostListView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/me/posts [GET]
func (s *Server) ListMyPosts() gin.HandlerFunc {
	return func(c *gin.Context) {
		limit, offset, err := parsePagination(c)
		if err != nil {
			Return(c, nil, err)

			return
		}

		items, total, err := s.dailyPost.History(c.Request.Context(), currentUserID(c), limit, offset)
		if err != nil {
			Return(c, nil, err)

			return
		}

		views := make([]dailyPostItemView, 0, len(items))
		for i := range items {
			views = append(views, dailyPostItemView{Day: items[i].Day.Format(time.DateOnly), Post: toPostView(items[i].Post)})
		}

		Return(c, dailyPostListView{Items: views, Total: total, Limit: limit, Offset: offset}, nil)
	}
}

// postFilter reads the shared list query parameters; on a bad value it answers and returns false.
func postFilter(c *gin.Context) (domain.PostFilter, bool) {
	f, ok := articleFilter(c)

	return domain.PostFilter{
		Age: f.Age, GoalID: f.GoalID, TopicID: f.TopicID, Search: f.Search, MaxMinutes: float64(f.MaxMinutes),
		Limit: f.Limit, Offset: f.Offset,
	}, ok
}

type postView struct {
	ID      string `json:"id" example:"7b0c1f1e-2d7a-4d8e-9a55-0f4a0d7f9c11"`
	TitleUz string `json:"title_uz" example:"Ranglar qo‘shig‘i"`
	TitleRu string `json:"title_ru" example:"Песня о цветах"`
	TitleEn string `json:"title_en" example:"The colours song"`
	VideoID string `json:"video_id" example:"3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f"`
	// VideoURL plays the video without a token: it redirects (302) to a short-lived link.
	VideoURL string `json:"video_url" example:"/v1/media/3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f"`
	// PhotoID is the cover picture, a media id; null without one.
	PhotoID *string `json:"photo_id" example:"5d2f8a90-1c3b-4e6f-8a7d-9b0c1d2e3f4a"`
	// PhotoURL is where to load the cover from (GET, no token); null without one.
	PhotoURL *string `json:"photo_url" example:"/v1/media/5d2f8a90-1c3b-4e6f-8a7d-9b0c1d2e3f4a"`
	GoalID   string  `json:"goal_id" example:"2b6f0cc9-0f3e-4b1a-9a7e-5d8c3e2f1a00"`
	// TopicID is the optional topic tag from GET /v1/topics; null means none.
	TopicID *string `json:"topic_id" example:"9a1c7e52-3b4d-4f6a-8e2b-7c5d1f0a9b3e"`
	MinAge  int     `json:"min_age" example:"3"`
	MaxAge  int     `json:"max_age" example:"5"`
	// Duration is in minutes, a decimal: 4.2 is 4:12.
	Duration     float64   `json:"duration" example:"4.2"`
	SuggestionUz string    `json:"suggestion_uz"`
	SuggestionRu string    `json:"suggestion_ru"`
	SuggestionEn string    `json:"suggestion_en"`
	QuestionsUz  string    `json:"questions_uz"`
	QuestionsRu  string    `json:"questions_ru"`
	QuestionsEn  string    `json:"questions_en"`
	Status       string    `json:"status" example:"published" enums:"published,in_review,unpublished"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// postTeaserView is a post without its video and texts, for locked or browsing lists.
type postTeaserView struct {
	ID      string `json:"id" example:"7b0c1f1e-2d7a-4d8e-9a55-0f4a0d7f9c11"`
	TitleUz string `json:"title_uz" example:"Hayvonlar ovozi"`
	TitleRu string `json:"title_ru" example:"Голоса животных"`
	TitleEn string `json:"title_en" example:"Animal sounds"`
	GoalID  string `json:"goal_id" example:"2b6f0cc9-0f3e-4b1a-9a7e-5d8c3e2f1a00"`
	// TopicID is the optional topic tag from GET /v1/topics; null means none.
	TopicID  *string `json:"topic_id" example:"9a1c7e52-3b4d-4f6a-8e2b-7c5d1f0a9b3e"`
	MinAge   int     `json:"min_age" example:"3"`
	MaxAge   int     `json:"max_age" example:"5"`
	Duration float64 `json:"duration" example:"3.67"`
	// Locked is true for a user without Birga Pro when the post does not open for them: on
	// GET /v1/posts the posts that were never their daily video, and every "next" video.
	Locked bool `json:"locked" example:"true"`
}

type postListView struct {
	Items  []postView `json:"items"`
	Total  int        `json:"total" example:"42"`
	Limit  int        `json:"limit" example:"20"`
	Offset int        `json:"offset" example:"0"`
}

type postTeaserListView struct {
	Items  []postTeaserView `json:"items"`
	Total  int              `json:"total" example:"42"`
	Limit  int              `json:"limit" example:"20"`
	Offset int              `json:"offset" example:"0"`
}

type dailyPostView struct {
	// Day is today's date in Uzbekistan time.
	Day  string           `json:"day" example:"2026-10-19"`
	Post postView         `json:"post"`
	Next []postTeaserView `json:"next"`
}

type dailyPostItemView struct {
	Day  string   `json:"day" example:"2026-10-19"`
	Post postView `json:"post"`
}

type dailyPostListView struct {
	Items  []dailyPostItemView `json:"items"`
	Total  int                 `json:"total" example:"12"`
	Limit  int                 `json:"limit" example:"20"`
	Offset int                 `json:"offset" example:"0"`
}

func toPostView(p domain.Post) postView {
	return postView{
		ID:           p.ID,
		TitleUz:      p.TitleUz,
		TitleRu:      p.TitleRu,
		TitleEn:      p.TitleEn,
		VideoID:      p.VideoID,
		VideoURL:     mediaPath(p.VideoID),
		PhotoID:      p.PhotoID,
		PhotoURL:     photoURL(p.PhotoID),
		GoalID:       p.GoalID,
		TopicID:      p.TopicID,
		MinAge:       p.MinAge,
		MaxAge:       p.MaxAge,
		Duration:     p.Duration,
		SuggestionUz: p.SuggestionUz,
		SuggestionRu: p.SuggestionRu,
		SuggestionEn: p.SuggestionEn,
		QuestionsUz:  p.QuestionsUz,
		QuestionsRu:  p.QuestionsRu,
		QuestionsEn:  p.QuestionsEn,
		Status:       p.Status,
		CreatedAt:    p.CreatedAt,
		UpdatedAt:    p.UpdatedAt,
	}
}

func toPostTeaserView(p domain.Post) postTeaserView {
	return postTeaserView{
		ID: p.ID, TitleUz: p.TitleUz, TitleRu: p.TitleRu, TitleEn: p.TitleEn,
		GoalID:  p.GoalID,
		TopicID: p.TopicID, MinAge: p.MinAge, MaxAge: p.MaxAge, Duration: p.Duration, Locked: p.Locked,
	}
}
