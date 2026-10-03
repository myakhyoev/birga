package rest

import (
	"time"

	"github.com/gin-gonic/gin"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	activityrecommender "gitlab.com/loyihalar/birga/backend/internal/usecases/activity_recommender"
	completionrecorder "gitlab.com/loyihalar/birga/backend/internal/usecases/completion_recorder"
)

// RecommendActivity godoc swagger
// @Summary today's recommended activity for a child
// @Description - only the child's parents can ask (404 otherwise); the child's age is clamped to 2..6
// @Description - goal and minutes narrow the choice; minutes keeps activities with duration_minutes <= minutes
// @Description - activities the child has never done come first, then the one done longest ago;
// @Description   ties are broken per child and per day, so the pick stays the same all day until it is done
// @Description - 404 when no published activity matches
// @Tags children
// @Security BearerAuth
// @Produce json
// @Param id path string true "child id (UUID)"
// @Param goal query string false "development goal" Enums(language,motor,cognitive,social,emotional)
// @Param minutes query int false "time available, in minutes"
// @Success 200 {object} rest.R{data=rest.activityView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/children/{id}/recommendation [GET]
func (s *Server) RecommendActivity() gin.HandlerFunc {
	return func(c *gin.Context) {
		childID := c.Param("id")
		if err := ValidateUUID(childID); err != nil {
			Return(c, nil, err)

			return
		}

		minutes, err := parseOptionalInt(c, "minutes")
		if err != nil {
			Return(c, nil, err)

			return
		}

		a, err := s.activityRecommender.Execute(c.Request.Context(), activityrecommender.Request{
			UserID:     currentUserID(c),
			ChildID:    childID,
			Goal:       c.Query("goal"),
			MaxMinutes: minutes,
		})
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toActivityView(a), nil)
	}
}

// CompleteActivityRequest is the body of POST /v1/children/{id}/completions.
type CompleteActivityRequest struct {
	ActivityID string  `json:"activity_id" example:"7b0c1f1e-2d7a-4d8e-9a55-0f4a0d7f9c11"`
	Note       *string `json:"note" example:"Qizil rangni birinchi topdi"` // caregiver's reflection, optional
}

// CompleteActivity godoc swagger
// @Summary marks an activity done for a child today
// @Description - only the child's parents can record (404 otherwise); the activity must be published (404)
// @Description - the day is the current date in Uzbekistan (UTC+5); repeating the same activity on the same
// @Description   day returns the existing completion, replacing its note if a new one is sent
// @Description - note is the caregiver's optional reflection, at most 1000 characters (422)
// @Tags children
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "child id (UUID)"
// @Param body body CompleteActivityRequest true "completion"
// @Success 200 {object} rest.R{data=rest.completionView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/children/{id}/completions [POST]
func (s *Server) CompleteActivity() gin.HandlerFunc {
	return func(c *gin.Context) {
		childID := c.Param("id")
		if err := ValidateUUID(childID); err != nil {
			Return(c, nil, err)

			return
		}

		var req CompleteActivityRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Return(c, nil, errs.Errf(errs.ErrBadRequest, "invalid JSON body: %s", err.Error()))

			return
		}

		if err := ValidateUUID(req.ActivityID); err != nil {
			Return(c, nil, errs.Errf(errs.ErrValidation, "activity_id must be a UUID"))

			return
		}

		done, err := s.completionRecorder.Execute(c.Request.Context(), completionrecorder.Request{
			UserID:     currentUserID(c),
			ChildID:    childID,
			ActivityID: req.ActivityID,
			Note:       req.Note,
		})
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toCompletionView(done), nil)
	}
}

// ListCompletions godoc swagger
// @Summary lists a child's completions, newest first
// @Tags children
// @Security BearerAuth
// @Produce json
// @Param id path string true "child id (UUID)"
// @Param limit query int false "page size (default 20, max 100)"
// @Param offset query int false "offset"
// @Success 200 {object} rest.R{data=rest.completionListView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/children/{id}/completions [GET]
func (s *Server) ListCompletions() gin.HandlerFunc {
	return func(c *gin.Context) {
		childID := c.Param("id")
		if err := ValidateUUID(childID); err != nil {
			Return(c, nil, err)

			return
		}

		limit, offset, err := parsePagination(c)
		if err != nil {
			Return(c, nil, err)

			return
		}

		items, total, err := s.completionLister.Execute(c.Request.Context(), currentUserID(c),
			domain.CompletionFilter{ChildID: childID, Limit: limit, Offset: offset})
		if err != nil {
			Return(c, nil, err)

			return
		}

		views := make([]completionView, 0, len(items))
		for i := range items {
			views = append(views, toCompletionView(items[i]))
		}

		Return(c, completionListView{Items: views, Total: total, Limit: limit, Offset: offset}, nil)
	}
}

// GetStreak godoc swagger
// @Summary a child's streak and progress
// @Description - current: consecutive days with a completion up to today, or up to yesterday while
// @Description   today has none yet (the streak is not lost until a whole day is missed)
// @Description - longest: the longest such run; this_week: days with a completion since Monday
// @Description - days are counted in Uzbekistan time (UTC+5)
// @Tags children
// @Security BearerAuth
// @Produce json
// @Param id path string true "child id (UUID)"
// @Success 200 {object} rest.R{data=rest.streakView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/children/{id}/streak [GET]
func (s *Server) GetStreak() gin.HandlerFunc {
	return func(c *gin.Context) {
		childID := c.Param("id")
		if err := ValidateUUID(childID); err != nil {
			Return(c, nil, err)

			return
		}

		st, err := s.streakGetter.Execute(c.Request.Context(), currentUserID(c), childID)
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toStreakView(st), nil)
	}
}

type completionView struct {
	ID          string    `json:"id" example:"5d2f8a90-1c3b-4e6f-8a7d-9b0c1d2e3f4a"`
	ChildID     string    `json:"child_id" example:"3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f"`
	ActivityID  string    `json:"activity_id" example:"7b0c1f1e-2d7a-4d8e-9a55-0f4a0d7f9c11"`
	UserID      *string   `json:"user_id" example:"9a8b7c6d-5e4f-4a3b-2c1d-0e9f8a7b6c5d"`
	CompletedOn string    `json:"completed_on" example:"2026-10-03"`
	Note        *string   `json:"note" example:"Qizil rangni birinchi topdi"`
	CreatedAt   time.Time `json:"created_at"`
}

type completionListView struct {
	Items  []completionView `json:"items"`
	Total  int              `json:"total" example:"12"`
	Limit  int              `json:"limit" example:"20"`
	Offset int              `json:"offset" example:"0"`
}

type streakView struct {
	Current         int     `json:"current" example:"3"`
	Longest         int     `json:"longest" example:"7"`
	CompletedToday  bool    `json:"completed_today" example:"true"`
	ThisWeek        int     `json:"this_week" example:"2"`
	Total           int     `json:"total" example:"12"`
	LastCompletedOn *string `json:"last_completed_on" example:"2026-10-03"`
}

func toCompletionView(c domain.Completion) completionView {
	return completionView{
		ID:          c.ID,
		ChildID:     c.ChildID,
		ActivityID:  c.ActivityID,
		UserID:      c.UserID,
		CompletedOn: c.CompletedOn.Format(time.DateOnly),
		Note:        c.Note,
		CreatedAt:   c.CreatedAt,
	}
}

func toStreakView(s domain.Streak) streakView {
	v := streakView{
		Current:        s.Current,
		Longest:        s.Longest,
		CompletedToday: s.CompletedToday,
		ThisWeek:       s.ThisWeek,
		Total:          s.Total,
	}

	if s.LastCompletedOn != nil {
		last := s.LastCompletedOn.Format(time.DateOnly)
		v.LastCompletedOn = &last
	}

	return v
}

// CreateChildRequest is the body of POST /v1/children.
type CreateChildRequest struct {
	Name    string  `json:"name" example:"Amir"`
	Age     *int    `json:"age" example:"4"`
	Gender  string  `json:"gender" example:"male" enums:"male,female"`
	PhotoID *string `json:"photo_id" example:"3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f"`
}

// CreateChild godoc swagger
// @Summary adds a child to the signed-in user
// @Description - creates the child profile and links it to the caller in the same transaction;
// @Description   the caller is the child's first parent
// @Description - any signed-in role (user, paid_user, admin) may add children
// @Description - name is required, at most 100 characters; age is required, 0..18; gender is male or female
// @Description - photo_id is an optional id from POST /v1/media (422 if unknown)
// @Tags children
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body CreateChildRequest true "child"
// @Success 200 {object} rest.R{data=rest.childView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 403 {object} rest.ForbiddenResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/children [POST]
func (s *Server) CreateChild() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreateChildRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Return(c, nil, errs.Errf(errs.ErrBadRequest, "invalid JSON body: %s", err.Error()))

			return
		}

		if req.Age == nil {
			Return(c, nil, errs.Errf(errs.ErrValidation, "age is required"))

			return
		}

		child, err := s.childCreator.Execute(c.Request.Context(), currentUserID(c), domain.Child{
			Name:    req.Name,
			Age:     *req.Age,
			Gender:  req.Gender,
			PhotoID: req.PhotoID,
		})
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toChildView(child), nil)
	}
}

type childView struct {
	ID        string    `json:"id" example:"3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f"`
	Name      string    `json:"name" example:"Amir"`
	Age       int       `json:"age" example:"4"`
	Gender    string    `json:"gender" example:"male"`
	PhotoID   *string   `json:"photo_id" example:"5d2f8a90-1c3b-4e6f-8a7d-9b0c1d2e3f4a"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toChildView(c domain.Child) childView {
	return childView{
		ID:        c.ID,
		Name:      c.Name,
		Age:       c.Age,
		Gender:    c.Gender,
		PhotoID:   c.PhotoID,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}
