package rest

import (
	"time"

	"github.com/gin-gonic/gin"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

// CreateGoalRequest is the body of POST /v1/admin/goals.
type CreateGoalRequest struct {
	NameUz string `json:"name_uz" example:"Nutqni rivojlantirish"`
	NameRu string `json:"name_ru" example:"Развитие речи"`
	NameEn string `json:"name_en" example:"Speech development"`
}

// UpdateGoalRequest is the body of PATCH /v1/admin/goals/{id}. Omitted or null fields are left unchanged.
type UpdateGoalRequest struct {
	NameUz *string `json:"name_uz" example:"Nutqni rivojlantirish"`
	NameRu *string `json:"name_ru" example:"Развитие речи"`
	NameEn *string `json:"name_en" example:"Speech development"`
}

// ListGoals godoc swagger
// @Summary lists the goals a user can pick at sign-up
// @Description - open without a token; every active goal, oldest first, not paginated
// @Tags goals
// @Produce json
// @Success 200 {object} rest.R{data=rest.goalListView}
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/goals [GET]
func (s *Server) ListGoals() gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := s.goalLister.Execute(c.Request.Context())
		if err != nil {
			Return(c, nil, err)

			return
		}

		views := make([]goalView, 0, len(items))
		for i := range items {
			views = append(views, toGoalView(items[i]))
		}

		Return(c, goalListView{Items: views, Total: len(views)}, nil)
	}
}

// GetGoal godoc swagger
// @Summary returns one goal
// @Description - open without a token; a deleted goal is 404
// @Tags goals
// @Produce json
// @Param id path string true "goal id (UUID)"
// @Success 200 {object} rest.R{data=rest.goalView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/goals/{id} [GET]
func (s *Server) GetGoal() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := ValidateUUID(id); err != nil {
			Return(c, nil, err)

			return
		}

		g, err := s.goalGetter.Execute(c.Request.Context(), id)
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toGoalView(g), nil)
	}
}

// CreateGoal godoc swagger
// @Summary creates a goal
// @Description - name_uz, name_ru and name_en are required, trimmed, at most 100 characters each (422)
// @Description - each name is unique among active goals in its language, ignoring case (409)
// @Tags admin
// @Security AdminKey
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body CreateGoalRequest true "goal"
// @Success 200 {object} rest.R{data=rest.goalView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 403 {object} rest.ForbiddenResponse
// @Failure 409 {object} rest.ConflictResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/admin/goals [POST]
func (s *Server) CreateGoal() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreateGoalRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Return(c, nil, errs.Errf(errs.ErrBadRequest, "invalid JSON body: %s", err.Error()))

			return
		}

		g, err := s.goalCreator.Execute(c.Request.Context(), domain.Goal{NameUz: req.NameUz, NameRu: req.NameRu, NameEn: req.NameEn})
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toGoalView(g), nil)
	}
}

// UpdateGoal godoc swagger
// @Summary renames a goal
// @Description - send only the names to change; the same rules as create apply (422, 409)
// @Tags admin
// @Security AdminKey
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "goal id (UUID)"
// @Param body body UpdateGoalRequest true "names to change"
// @Success 200 {object} rest.R{data=rest.goalView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 403 {object} rest.ForbiddenResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 409 {object} rest.ConflictResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/admin/goals/{id} [PATCH]
func (s *Server) UpdateGoal() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := ValidateUUID(id); err != nil {
			Return(c, nil, err)

			return
		}

		var req UpdateGoalRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Return(c, nil, errs.Errf(errs.ErrBadRequest, "invalid JSON body: %s", err.Error()))

			return
		}

		g, err := s.goalUpdater.Execute(c.Request.Context(), id,
			domain.GoalUpdate{NameUz: req.NameUz, NameRu: req.NameRu, NameEn: req.NameEn})
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toGoalView(g), nil)
	}
}

// DeleteGoal godoc swagger
// @Summary soft-deletes a goal
// @Description - sets deleted_at: the goal disappears from /v1/goals, sign-up refuses its id, and its
// @Description   id is removed from every user's goal_ids; its names can be used again
// @Tags admin
// @Security AdminKey
// @Security BearerAuth
// @Produce json
// @Param id path string true "goal id (UUID)"
// @Success 200 {object} rest.R
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 403 {object} rest.ForbiddenResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/admin/goals/{id} [DELETE]
func (s *Server) DeleteGoal() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := ValidateUUID(id); err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, nil, s.goalDeleter.Execute(c.Request.Context(), id))
	}
}

type goalView struct {
	ID        string    `json:"id" example:"2b6f0cc9-0f3e-4b1a-9a7e-5d8c3e2f1a00"`
	NameUz    string    `json:"name_uz" example:"Nutqni rivojlantirish"`
	NameRu    string    `json:"name_ru" example:"Развитие речи"`
	NameEn    string    `json:"name_en" example:"Speech development"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type goalListView struct {
	Items []goalView `json:"items"`
	Total int        `json:"total" example:"6"`
}

func toGoalView(g domain.Goal) goalView {
	return goalView{
		ID:        g.ID,
		NameUz:    g.NameUz,
		NameRu:    g.NameRu,
		NameEn:    g.NameEn,
		CreatedAt: g.CreatedAt,
		UpdatedAt: g.UpdatedAt,
	}
}
