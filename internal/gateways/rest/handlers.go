package rest

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

const healthTimeout = 2 * time.Second

// Ping godoc swagger
// @Summary liveness probe
// @Tags system
// @Produce json
// @Success 200 {object} rest.R
// @Router /ping [GET]
func (s *Server) Ping() gin.HandlerFunc {
	return func(c *gin.Context) {
		Return(c, "Pong", nil)
	}
}

// Health godoc swagger
// @Summary readiness probe
// @Description Checks that the database is reachable.
// @Tags system
// @Produce json
// @Success 200 {object} rest.R
// @Failure 503 {object} rest.ServiceUnavailableResponse
// @Router /health [GET]
func (s *Server) Health() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), healthTimeout)
		defer cancel()

		if err := s.health.Ping(ctx); err != nil {
			logger.WithContext(s.l, ctx).Error("health.Ping", zap.Error(err))
			fail(c, http.StatusServiceUnavailable, _errCodeUnavailable, "database or redis unavailable")

			return
		}

		Return(c, "OK", nil)
	}
}

// CreateActivityRequest is the body of POST /v1/admin/activities.
type CreateActivityRequest struct {
	TitleUz         string   `json:"title_uz" example:"Rangli toshlar"`
	TitleRu         string   `json:"title_ru" example:"Цветные камни"`
	DescriptionUz   string   `json:"description_uz" example:"Toshlarni rangi bo'yicha saralang"`
	DescriptionRu   string   `json:"description_ru" example:"Сортируйте камни по цвету"`
	GoalIDs         []string `json:"goal_ids" example:"2b6f0cc9-0f3e-4b1a-9a7e-5d8c3e2f1a00"`
	MinAge          int      `json:"min_age" example:"3" minimum:"2" maximum:"6"`
	MaxAge          int      `json:"max_age" example:"5" minimum:"2" maximum:"6"`
	DurationMinutes int      `json:"duration_minutes" example:"10" minimum:"1" maximum:"60"`
	IsPublished     bool     `json:"is_published" example:"true"`
}

// CreateActivity godoc swagger
// @Summary creates an activity
// @Description - title/description are required in both Uzbek (uz) and Russian (ru)
// @Description - goal_ids lists 1 to 5 goals the activity serves, ids from GET /v1/goals; each must be an
// @Description   active goal (422), repeats are stored once
// @Description - min_age/max_age are child ages in years, within 2..6
// @Tags admin
// @Security AdminKey
// @Accept json
// @Produce json
// @Param body body CreateActivityRequest true "activity"
// @Success 200 {object} rest.R{data=rest.activityView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/admin/activities [POST]
func (s *Server) CreateActivity() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreateActivityRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Return(c, nil, errs.Errf(errs.ErrBadRequest, "invalid JSON body: %s", err.Error()))

			return
		}

		a, err := s.activityCreator.Execute(c.Request.Context(), domain.Activity{
			TitleUz:         req.TitleUz,
			TitleRu:         req.TitleRu,
			DescriptionUz:   req.DescriptionUz,
			DescriptionRu:   req.DescriptionRu,
			GoalIDs:         req.GoalIDs,
			MinAge:          req.MinAge,
			MaxAge:          req.MaxAge,
			DurationMinutes: req.DurationMinutes,
			IsPublished:     req.IsPublished,
		})
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toActivityView(a), nil)
	}
}

// ListActivities godoc swagger
// @Summary lists published activities
// @Tags activities
// @Produce json
// @Param age query int false "child age in years (2..6)"
// @Param goal_id query string false "goal id (UUID) from GET /v1/goals"
// @Param limit query int false "page size (default 20, max 100)"
// @Param offset query int false "offset"
// @Success 200 {object} rest.R{data=rest.activityListView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/activities [GET]
func (s *Server) ListActivities() gin.HandlerFunc {
	return s.listActivities(true)
}

// AdminListActivities godoc swagger
// @Summary lists all activities, including unpublished
// @Tags admin
// @Security AdminKey
// @Produce json
// @Param age query int false "child age in years (2..6)"
// @Param goal_id query string false "goal id (UUID) from GET /v1/goals"
// @Param limit query int false "page size (default 20, max 100)"
// @Param offset query int false "offset"
// @Success 200 {object} rest.R{data=rest.activityListView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/admin/activities [GET]
func (s *Server) AdminListActivities() gin.HandlerFunc {
	return s.listActivities(false)
}

func (s *Server) listActivities(publishedOnly bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit, offset, err := parsePagination(c)
		if err != nil {
			Return(c, nil, err)

			return
		}

		age, err := parseOptionalInt(c, "age")
		if err != nil {
			Return(c, nil, err)

			return
		}

		items, total, err := s.activityLister.Execute(c.Request.Context(), domain.ActivityFilter{
			Age:           age,
			GoalID:        c.Query("goal_id"),
			PublishedOnly: publishedOnly,
			Limit:         limit,
			Offset:        offset,
		})
		if err != nil {
			Return(c, nil, err)

			return
		}

		views := make([]activityView, 0, len(items))
		for i := range items {
			views = append(views, toActivityView(items[i]))
		}

		Return(c, activityListView{Items: views, Total: total, Limit: limit, Offset: offset}, nil)
	}
}

// GetActivity godoc swagger
// @Summary returns a published activity
// @Tags activities
// @Produce json
// @Param id path string true "activity id (UUID)"
// @Success 200 {object} rest.R{data=rest.activityView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/activities/{id} [GET]
func (s *Server) GetActivity() gin.HandlerFunc {
	return s.getActivity(false)
}

// AdminGetActivity godoc swagger
// @Summary returns an activity, including unpublished
// @Tags admin
// @Security AdminKey
// @Produce json
// @Param id path string true "activity id (UUID)"
// @Success 200 {object} rest.R{data=rest.activityView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/admin/activities/{id} [GET]
func (s *Server) AdminGetActivity() gin.HandlerFunc {
	return s.getActivity(true)
}

func (s *Server) getActivity(includeUnpublished bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := ValidateUUID(id); err != nil {
			Return(c, nil, err)

			return
		}

		a, err := s.activityGetter.Execute(c.Request.Context(), id, includeUnpublished)
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toActivityView(a), nil)
	}
}

// UpdateActivityRequest is the body of PATCH /v1/admin/activities/{id}. Omitted or null fields are
// left unchanged.
type UpdateActivityRequest struct {
	TitleUz         *string  `json:"title_uz" example:"Rangli toshlar"`
	TitleRu         *string  `json:"title_ru" example:"Цветные камни"`
	DescriptionUz   *string  `json:"description_uz" example:"Toshlarni rangi bo'yicha saralang"`
	DescriptionRu   *string  `json:"description_ru" example:"Сортируйте камни по цвету"`
	GoalIDs         []string `json:"goal_ids" example:"2b6f0cc9-0f3e-4b1a-9a7e-5d8c3e2f1a00"`
	MinAge          *int     `json:"min_age" example:"3" minimum:"2" maximum:"6"`
	MaxAge          *int     `json:"max_age" example:"5" minimum:"2" maximum:"6"`
	DurationMinutes *int     `json:"duration_minutes" example:"10" minimum:"1" maximum:"60"`
	IsPublished     *bool    `json:"is_published" example:"true"`
}

// UpdateActivity godoc swagger
// @Summary updates an activity partially, including publishing or unpublishing it
// @Description - only the fields present in the body change; omitted or null fields are kept
// @Description - same rules as create; texts cannot be emptied; the resulting age range must stay valid (422)
// @Description - goal_ids, when sent, replaces the whole list and must hold 1 to 5 active goal ids
// @Description - send {"is_published": true} to publish, false to hide it from the app
// @Tags admin
// @Security AdminKey
// @Accept json
// @Produce json
// @Param id path string true "activity id (UUID)"
// @Param body body UpdateActivityRequest true "fields to change"
// @Success 200 {object} rest.R{data=rest.activityView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/admin/activities/{id} [PATCH]
func (s *Server) UpdateActivity() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := ValidateUUID(id); err != nil {
			Return(c, nil, err)

			return
		}

		var req UpdateActivityRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Return(c, nil, errs.Errf(errs.ErrBadRequest, "invalid JSON body: %s", err.Error()))

			return
		}

		a, err := s.activityUpdater.Execute(c.Request.Context(), id, domain.ActivityUpdate{
			TitleUz:         req.TitleUz,
			TitleRu:         req.TitleRu,
			DescriptionUz:   req.DescriptionUz,
			DescriptionRu:   req.DescriptionRu,
			GoalIDs:         req.GoalIDs,
			MinAge:          req.MinAge,
			MaxAge:          req.MaxAge,
			DurationMinutes: req.DurationMinutes,
			IsPublished:     req.IsPublished,
		})
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toActivityView(a), nil)
	}
}

// DeleteActivity godoc swagger
// @Summary soft-deletes an activity
// @Description Sets deleted_at; the activity disappears from every list, get and recommendation.
// @Description Completions that point at it are kept.
// @Tags admin
// @Security AdminKey
// @Produce json
// @Param id path string true "activity id (UUID)"
// @Success 200 {object} rest.R
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/admin/activities/{id} [DELETE]
func (s *Server) DeleteActivity() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := ValidateUUID(id); err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, nil, s.activityDeleter.Execute(c.Request.Context(), id))
	}
}

type activityView struct {
	ID              string    `json:"id" example:"7b0c1f1e-2d7a-4d8e-9a55-0f4a0d7f9c11"`
	TitleUz         string    `json:"title_uz" example:"Rangli toshlar"`
	TitleRu         string    `json:"title_ru" example:"Цветные камни"`
	DescriptionUz   string    `json:"description_uz"`
	DescriptionRu   string    `json:"description_ru"`
	GoalIDs         []string  `json:"goal_ids" example:"2b6f0cc9-0f3e-4b1a-9a7e-5d8c3e2f1a00"`
	MinAge          int       `json:"min_age" example:"3"`
	MaxAge          int       `json:"max_age" example:"5"`
	DurationMinutes int       `json:"duration_minutes" example:"10"`
	IsPublished     bool      `json:"is_published" example:"true"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type activityListView struct {
	Items  []activityView `json:"items"`
	Total  int            `json:"total" example:"42"`
	Limit  int            `json:"limit" example:"20"`
	Offset int            `json:"offset" example:"0"`
}

func toActivityView(a domain.Activity) activityView {
	return activityView{
		ID:              a.ID,
		TitleUz:         a.TitleUz,
		TitleRu:         a.TitleRu,
		DescriptionUz:   a.DescriptionUz,
		DescriptionRu:   a.DescriptionRu,
		GoalIDs:         a.GoalIDs,
		MinAge:          a.MinAge,
		MaxAge:          a.MaxAge,
		DurationMinutes: a.DurationMinutes,
		IsPublished:     a.IsPublished,
		CreatedAt:       a.CreatedAt,
		UpdatedAt:       a.UpdatedAt,
	}
}
