package rest

import (
	"time"

	"github.com/gin-gonic/gin"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

// CreateArticleRequest is the body of POST /v1/admin/articles.
type CreateArticleRequest struct {
	TitleUz       string  `json:"title_uz" example:"Nega bola «yo‘q» deydi?"`
	TitleRu       string  `json:"title_ru" example:"Почему ребёнок говорит «нет»?"`
	TitleEn       string  `json:"title_en" example:"Why does a child say \"no\"?"`
	ArticleBodyUz string  `json:"article_body_uz" example:"2–5 yoshda «yo‘q» deyish rivojlanishning odatiy bosqichi."`
	ArticleBodyRu string  `json:"article_body_ru" example:"В 2–5 лет «нет» — обычный этап развития."`
	ArticleBodyEn string  `json:"article_body_en" example:"Saying \"no\" at 2–5 is a normal stage of development."`
	TipUz         string  `json:"tip_uz" example:"Bugun kamida 3 marta bolaga ikki variantli tanlov bering."`
	TipRu         string  `json:"tip_ru" example:"Сегодня хотя бы 3 раза дайте ребёнку выбор из двух вариантов."`
	TipEn         string  `json:"tip_en" example:"Today, offer your child a choice of two options at least 3 times."`
	PhotoID       *string `json:"photo_id" example:"3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f"`
	// ExpectedReadTime is in minutes.
	ExpectedReadTime int    `json:"expected_read_time" example:"3" minimum:"1" maximum:"60"`
	MinAge           int    `json:"min_age" example:"2" minimum:"2" maximum:"6"`
	MaxAge           int    `json:"max_age" example:"5" minimum:"2" maximum:"6"`
	GoalID           string `json:"goal_id" example:"2b6f0cc9-0f3e-4b1a-9a7e-5d8c3e2f1a00"`
	// TopicID is the optional topic tag from GET /v1/topics; null means none.
	TopicID *string `json:"topic_id" example:"9a1c7e52-3b4d-4f6a-8e2b-7c5d1f0a9b3e"`
	Status  string  `json:"status" example:"in_review" enums:"published,in_review,unpublished"`
	Source  string  `json:"source" example:"admin" enums:"test,admin,harvard,ai"`
}

// CreateArticle godoc swagger
// @Summary creates an article
// @Description - title and article_body are required in Uzbek, Russian and English; tip_* is optional ("" = no tip box)
// @Description - goal_id is one active goal from GET /v1/goals (422 if unknown)
// @Description - topic_id is an optional topic from GET /v1/topics, shown as the tag instead of the goal (422 if unknown)
// @Description - photo_id is an optional id from POST /v1/media (422 if unknown)
// @Description - expected_read_time is in minutes (1..60); min_age/max_age are child ages within 2..6
// @Description - status is published, in_review or unpublished; omitted means in_review. Only published articles reach the app
// @Description - source is where it comes from: test, admin, harvard or ai; omitted means admin
// @Tags admin
// @Security AdminKey
// @Accept json
// @Produce json
// @Param body body CreateArticleRequest true "article"
// @Success 200 {object} rest.R{data=rest.articleView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/admin/articles [POST]
func (s *Server) CreateArticle() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreateArticleRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Return(c, nil, errs.Errf(errs.ErrBadRequest, "invalid JSON body: %s", err.Error()))

			return
		}

		a, err := s.articleCreator.Execute(c.Request.Context(), domain.Article{
			TitleUz:     req.TitleUz,
			TitleRu:     req.TitleRu,
			TitleEn:     req.TitleEn,
			BodyUz:      req.ArticleBodyUz,
			BodyRu:      req.ArticleBodyRu,
			BodyEn:      req.ArticleBodyEn,
			TipUz:       req.TipUz,
			TipRu:       req.TipRu,
			TipEn:       req.TipEn,
			PhotoID:     req.PhotoID,
			ReadMinutes: req.ExpectedReadTime,
			MinAge:      req.MinAge,
			MaxAge:      req.MaxAge,
			GoalID:      req.GoalID,
			TopicID:     req.TopicID,
			Status:      req.Status,
			Source:      req.Source,
		})
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toArticleView(a), nil)
	}
}

// AdminListArticles godoc swagger
// @Summary lists articles in every status
// @Tags admin
// @Security AdminKey
// @Produce json
// @Param q query string false "text the title (any language) contains, ignoring case (at most 100 characters)"
// @Param status query string false "published, in_review or unpublished; omitted lists all" Enums(published, in_review, unpublished)
// @Param source query string false "only content of this source" Enums(test, admin, harvard, ai)
// @Param goal_id query string false "goal id (UUID) from GET /v1/goals"
// @Param topic_id query string false "topic id (UUID) from GET /v1/topics"
// @Param age query int false "child age in years (2..6)"
// @Param max_minutes query int false "at most this many minutes"
// @Param limit query int false "page size (default 20, max 100)"
// @Param offset query int false "offset"
// @Success 200 {object} rest.R{data=rest.articleListView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/admin/articles [GET]
func (s *Server) AdminListArticles() gin.HandlerFunc {
	return func(c *gin.Context) {
		f, ok := articleFilter(c)
		if !ok {
			return
		}

		f.Status = c.Query("status")
		f.Source = c.Query("source")

		items, total, err := s.articleLister.Execute(c.Request.Context(), f)
		if err != nil {
			Return(c, nil, err)

			return
		}

		views := make([]articleView, 0, len(items))
		for i := range items {
			views = append(views, toArticleView(items[i]))
		}

		Return(c, articleListView{Items: views, Total: total, Limit: f.Limit, Offset: f.Offset}, nil)
	}
}

// AdminGetArticle godoc swagger
// @Summary returns an article in any status
// @Tags admin
// @Security AdminKey
// @Produce json
// @Param id path string true "article id (UUID)"
// @Success 200 {object} rest.R{data=rest.articleView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/admin/articles/{id} [GET]
func (s *Server) AdminGetArticle() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := ValidateUUID(id); err != nil {
			Return(c, nil, err)

			return
		}

		a, err := s.articleGetter.Execute(c.Request.Context(), id, "")
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toArticleView(a), nil)
	}
}

// UpdateArticleRequest is the body of PATCH /v1/admin/articles/{id}. Omitted or null fields are
// left unchanged.
type UpdateArticleRequest struct {
	TitleUz       *string `json:"title_uz" example:"Nega bola «yo‘q» deydi?"`
	TitleRu       *string `json:"title_ru" example:"Почему ребёнок говорит «нет»?"`
	TitleEn       *string `json:"title_en" example:"Why does a child say \"no\"?"`
	ArticleBodyUz *string `json:"article_body_uz"`
	ArticleBodyRu *string `json:"article_body_ru"`
	ArticleBodyEn *string `json:"article_body_en"`
	TipUz         *string `json:"tip_uz"`
	TipRu         *string `json:"tip_ru"`
	TipEn         *string `json:"tip_en"`
	// PhotoID "" removes the photo.
	PhotoID          *string `json:"photo_id" example:"3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f"`
	ExpectedReadTime *int    `json:"expected_read_time" example:"3" minimum:"1" maximum:"60"`
	MinAge           *int    `json:"min_age" example:"2" minimum:"2" maximum:"6"`
	MaxAge           *int    `json:"max_age" example:"5" minimum:"2" maximum:"6"`
	GoalID           *string `json:"goal_id" example:"2b6f0cc9-0f3e-4b1a-9a7e-5d8c3e2f1a00"`
	// TopicID "" removes the topic; omitted or null keeps it.
	TopicID *string `json:"topic_id" example:"9a1c7e52-3b4d-4f6a-8e2b-7c5d1f0a9b3e"`
	Status  *string `json:"status" example:"published" enums:"published,in_review,unpublished"`
	Source  *string `json:"source" example:"harvard" enums:"test,admin,harvard,ai"`
}

// UpdateArticle godoc swagger
// @Summary updates an article partially, including its status
// @Description - only the fields present in the body change; omitted or null fields are kept
// @Description - same rules as create; titles and bodies cannot be emptied; tip_* "" removes the tip; photo_id "" removes the photo
// @Description - the resulting age range must stay valid (422)
// @Description - send {"status": "published"} to show it in the app, "in_review" or "unpublished" to hide it
// @Tags admin
// @Security AdminKey
// @Accept json
// @Produce json
// @Param id path string true "article id (UUID)"
// @Param body body UpdateArticleRequest true "fields to change"
// @Success 200 {object} rest.R{data=rest.articleView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/admin/articles/{id} [PATCH]
func (s *Server) UpdateArticle() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := ValidateUUID(id); err != nil {
			Return(c, nil, err)

			return
		}

		var req UpdateArticleRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Return(c, nil, errs.Errf(errs.ErrBadRequest, "invalid JSON body: %s", err.Error()))

			return
		}

		a, err := s.articleUpdater.Execute(c.Request.Context(), id, domain.ArticleUpdate{
			TitleUz:     req.TitleUz,
			TitleRu:     req.TitleRu,
			TitleEn:     req.TitleEn,
			BodyUz:      req.ArticleBodyUz,
			BodyRu:      req.ArticleBodyRu,
			BodyEn:      req.ArticleBodyEn,
			TipUz:       req.TipUz,
			TipRu:       req.TipRu,
			TipEn:       req.TipEn,
			PhotoID:     req.PhotoID,
			ReadMinutes: req.ExpectedReadTime,
			MinAge:      req.MinAge,
			MaxAge:      req.MaxAge,
			GoalID:      req.GoalID,
			TopicID:     req.TopicID,
			Status:      req.Status,
			Source:      req.Source,
		})
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toArticleView(a), nil)
	}
}

// DeleteArticle godoc swagger
// @Summary soft-deletes an article
// @Description Sets deleted_at; the article disappears from every list, get, daily article and article history.
// @Tags admin
// @Security AdminKey
// @Produce json
// @Param id path string true "article id (UUID)"
// @Success 200 {object} rest.R
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/admin/articles/{id} [DELETE]
func (s *Server) DeleteArticle() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := ValidateUUID(id); err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, nil, s.articleDeleter.Execute(c.Request.Context(), id))
	}
}

// ListArticles godoc swagger
// @Summary lists published articles
// @Description Each item says whether the signed-in user marked it read (is_read). Without Birga Pro, articles that
// @Description were never the user's daily article have locked=true.
// @Tags articles
// @Security BearerAuth
// @Produce json
// @Param q query string false "text the title (any language) contains, ignoring case (at most 100 characters)"
// @Param goal_id query string false "goal id (UUID) from GET /v1/goals"
// @Param topic_id query string false "topic id (UUID) from GET /v1/topics"
// @Param age query int false "child age in years (2..6)"
// @Param max_minutes query int false "at most this many minutes"
// @Param limit query int false "page size (default 20, max 100)"
// @Param offset query int false "offset"
// @Success 200 {object} rest.R{data=rest.userArticleListView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/articles [GET]
func (s *Server) ListArticles() gin.HandlerFunc {
	return func(c *gin.Context) {
		f, ok := articleFilter(c)
		if !ok {
			return
		}

		f.Status = domain.ArticleStatusPublished
		f.ReaderID = currentUserID(c)

		if !currentUserPro(c) {
			f.LockFor = currentUserID(c)
		}

		items, total, err := s.articleLister.Execute(c.Request.Context(), f)
		if err != nil {
			Return(c, nil, err)

			return
		}

		views := make([]userArticleView, 0, len(items))
		for i := range items {
			views = append(views, toUserArticleView(items[i]))
		}

		Return(c, userArticleListView{Items: views, Total: total, Limit: f.Limit, Offset: f.Offset}, nil)
	}
}

// GetArticle godoc swagger
// @Summary returns a published article
// @Description Without Birga Pro only the user's daily articles of today or earlier open (403 otherwise).
// @Tags articles
// @Security BearerAuth
// @Produce json
// @Param id path string true "article id (UUID)"
// @Success 200 {object} rest.R{data=rest.userArticleView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/articles/{id} [GET]
func (s *Server) GetArticle() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := ValidateUUID(id); err != nil {
			Return(c, nil, err)

			return
		}

		a, err := s.articleGetter.ForUser(c.Request.Context(), currentUserID(c), currentUserPro(c), id)
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toUserArticleView(a), nil)
	}
}

// MarkArticleRead godoc swagger
// @Summary marks a published article read ("O‘qib chiqdim")
// @Description Calling it again is fine and changes nothing. The article then has is_read=true for this user,
// @Description and the daily article prefers articles the user has not read.
// @Tags articles
// @Security BearerAuth
// @Produce json
// @Param id path string true "article id (UUID)"
// @Success 200 {object} rest.R
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/articles/{id}/read [POST]
func (s *Server) MarkArticleRead() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := ValidateUUID(id); err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, nil, s.articleReader.Execute(c.Request.Context(), currentUserID(c), id))
	}
}

// GetDailyArticle godoc swagger
// @Summary returns the signed-in user's article of the day, and tomorrow's
// @Description The article is picked once per day (Uzbekistan time) and stays the same all day.
// @Description It prefers, in order: an article for one of the goals of the user's children, whose age range
// @Description fits one of the children, not shown or read before, then rotates through the goals.
// @Description tomorrow is the article that will show tomorrow ("Ertangi maqola"); null when there is no other article.
// @Description 404 when no article is published.
// @Tags articles
// @Security BearerAuth
// @Produce json
// @Success 200 {object} rest.R{data=rest.dailyArticleView}
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/me/articles/daily [GET]
func (s *Server) GetDailyArticle() gin.HandlerFunc {
	return func(c *gin.Context) {
		d, err := s.dailyArticle.Execute(c.Request.Context(), currentUserID(c))
		if err != nil {
			Return(c, nil, err)

			return
		}

		v := dailyArticleView{Day: d.Today.Day.Format(time.DateOnly), Article: toUserArticleView(d.Today.Article)}
		if d.Tomorrow != nil {
			t := toArticleTeaserView(*d.Tomorrow)
			v.Tomorrow = &t
		}

		Return(c, v, nil)
	}
}

// ListMyArticles godoc swagger
// @Summary lists the signed-in user's daily articles of earlier days and today, newest first
// @Description One item per day the user opened the daily article. Deleted articles are left out.
// @Tags articles
// @Security BearerAuth
// @Produce json
// @Param limit query int false "page size (default 20, max 100)"
// @Param offset query int false "offset"
// @Success 200 {object} rest.R{data=rest.dailyArticleListView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/me/articles [GET]
func (s *Server) ListMyArticles() gin.HandlerFunc {
	return func(c *gin.Context) {
		limit, offset, err := parsePagination(c)
		if err != nil {
			Return(c, nil, err)

			return
		}

		items, total, err := s.dailyArticle.History(c.Request.Context(), currentUserID(c), limit, offset)
		if err != nil {
			Return(c, nil, err)

			return
		}

		views := make([]dailyArticleItemView, 0, len(items))
		for i := range items {
			views = append(views, dailyArticleItemView{
				Day: items[i].Day.Format(time.DateOnly), Article: toUserArticleView(items[i].Article),
			})
		}

		Return(c, dailyArticleListView{Items: views, Total: total, Limit: limit, Offset: offset}, nil)
	}
}

// articleFilter reads the shared list query parameters; on a bad value it answers and returns false.
func articleFilter(c *gin.Context) (domain.ArticleFilter, bool) {
	limit, offset, err := parsePagination(c)
	if err != nil {
		Return(c, nil, err)

		return domain.ArticleFilter{}, false
	}

	age, err := parseOptionalInt(c, "age")
	if err != nil {
		Return(c, nil, err)

		return domain.ArticleFilter{}, false
	}

	maxMinutes, err := parseOptionalInt(c, "max_minutes")
	if err != nil {
		Return(c, nil, err)

		return domain.ArticleFilter{}, false
	}

	return domain.ArticleFilter{
		MaxMinutes: maxMinutes,
		Age:        age,
		GoalID:     c.Query("goal_id"),
		TopicID:    c.Query("topic_id"),
		Search:     c.Query("q"),
		Limit:      limit,
		Offset:     offset,
	}, true
}

type articleView struct {
	ID            string  `json:"id" example:"7b0c1f1e-2d7a-4d8e-9a55-0f4a0d7f9c11"`
	TitleUz       string  `json:"title_uz" example:"Nega bola «yo‘q» deydi?"`
	TitleRu       string  `json:"title_ru" example:"Почему ребёнок говорит «нет»?"`
	TitleEn       string  `json:"title_en" example:"Why does a child say \"no\"?"`
	ArticleBodyUz string  `json:"article_body_uz"`
	ArticleBodyRu string  `json:"article_body_ru"`
	ArticleBodyEn string  `json:"article_body_en"`
	TipUz         string  `json:"tip_uz"`
	TipRu         string  `json:"tip_ru"`
	TipEn         string  `json:"tip_en"`
	PhotoID       *string `json:"photo_id" example:"5d2f8a90-1c3b-4e6f-8a7d-9b0c1d2e3f4a"`
	// PhotoURL is where the app loads the photo from (GET, no token); null without a photo.
	PhotoURL *string `json:"photo_url" example:"/v1/media/5d2f8a90-1c3b-4e6f-8a7d-9b0c1d2e3f4a"`
	// ExpectedReadTime is in minutes.
	ExpectedReadTime int    `json:"expected_read_time" example:"3"`
	MinAge           int    `json:"min_age" example:"2"`
	MaxAge           int    `json:"max_age" example:"5"`
	GoalID           string `json:"goal_id" example:"2b6f0cc9-0f3e-4b1a-9a7e-5d8c3e2f1a00"`
	// TopicID is the optional topic tag from GET /v1/topics; null means none.
	TopicID   *string   `json:"topic_id" example:"9a1c7e52-3b4d-4f6a-8e2b-7c5d1f0a9b3e"`
	Status    string    `json:"status" example:"published" enums:"published,in_review,unpublished"`
	Source    string    `json:"source" example:"admin" enums:"test,admin,harvard,ai"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// userArticleView is an article as the app sees it.
type userArticleView struct {
	articleView
	// IsRead tells whether the signed-in user marked the article read.
	IsRead bool `json:"is_read" example:"false"`
	// Locked is true on GET /v1/articles for a user without Birga Pro when the article was never
	// their daily article: it does not open.
	Locked bool `json:"locked" example:"false"`
}

type articleListView struct {
	Items  []articleView `json:"items"`
	Total  int           `json:"total" example:"42"`
	Limit  int           `json:"limit" example:"20"`
	Offset int           `json:"offset" example:"0"`
}

type userArticleListView struct {
	Items  []userArticleView `json:"items"`
	Total  int               `json:"total" example:"42"`
	Limit  int               `json:"limit" example:"20"`
	Offset int               `json:"offset" example:"0"`
}

// articleTeaserView is the short form shown for tomorrow's article.
type articleTeaserView struct {
	ID               string `json:"id" example:"7b0c1f1e-2d7a-4d8e-9a55-0f4a0d7f9c11"`
	TitleUz          string `json:"title_uz" example:"Uyqudan oldingi tinch tartib"`
	TitleRu          string `json:"title_ru" example:"Спокойный ритуал перед сном"`
	TitleEn          string `json:"title_en" example:"A calm bedtime routine"`
	ExpectedReadTime int    `json:"expected_read_time" example:"4"`
}

type dailyArticleView struct {
	// Day is today's date in Uzbekistan time.
	Day      string             `json:"day" example:"2026-10-19"`
	Article  userArticleView    `json:"article"`
	Tomorrow *articleTeaserView `json:"tomorrow"`
}

type dailyArticleItemView struct {
	Day     string          `json:"day" example:"2026-10-19"`
	Article userArticleView `json:"article"`
}

type dailyArticleListView struct {
	Items  []dailyArticleItemView `json:"items"`
	Total  int                    `json:"total" example:"12"`
	Limit  int                    `json:"limit" example:"20"`
	Offset int                    `json:"offset" example:"0"`
}

func toArticleView(a domain.Article) articleView {
	return articleView{
		ID:               a.ID,
		TitleUz:          a.TitleUz,
		TitleRu:          a.TitleRu,
		TitleEn:          a.TitleEn,
		ArticleBodyUz:    a.BodyUz,
		ArticleBodyRu:    a.BodyRu,
		ArticleBodyEn:    a.BodyEn,
		TipUz:            a.TipUz,
		TipRu:            a.TipRu,
		TipEn:            a.TipEn,
		PhotoID:          a.PhotoID,
		PhotoURL:         photoURL(a.PhotoID),
		ExpectedReadTime: a.ReadMinutes,
		MinAge:           a.MinAge,
		MaxAge:           a.MaxAge,
		GoalID:           a.GoalID,
		TopicID:          a.TopicID,
		Status:           a.Status,
		Source:           a.Source,
		CreatedAt:        a.CreatedAt,
		UpdatedAt:        a.UpdatedAt,
	}
}

func toUserArticleView(a domain.Article) userArticleView {
	return userArticleView{articleView: toArticleView(a), IsRead: a.IsRead, Locked: a.Locked}
}

func toArticleTeaserView(a domain.Article) articleTeaserView {
	return articleTeaserView{
		ID: a.ID, TitleUz: a.TitleUz, TitleRu: a.TitleRu, TitleEn: a.TitleEn, ExpectedReadTime: a.ReadMinutes,
	}
}
