package rest

import (
	"time"

	"github.com/gin-gonic/gin"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	completionrecorder "gitlab.com/loyihalar/birga/backend/internal/usecases/completion_recorder"
)

// RecommendActivity godoc swagger
// @Summary today's plan activity for a child
// @Description - the activity of the plan day that is open today (status today in GET
// @Description   /v1/children/{id}/plan); the first call makes the child's first plan if it has none
// @Description - only the child's parents can ask (404 otherwise)
// @Description - 404 no plan day open today: today's plan day is already done, or the plan is done
// @Description   (start the next one with POST /v1/children/{id}/plan); 404 also when no published
// @Description   activity fits the child's age
// @Tags children
// @Security BearerAuth
// @Produce json
// @Param id path string true "child id (UUID)"
// @Success 200 {object} rest.R{data=rest.activityView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/children/{id}/recommendation [GET]
func (s *Server) RecommendActivity() gin.HandlerFunc {
	return func(c *gin.Context) {
		childID := c.Param("id")
		if err := ValidateUUID(childID); err != nil {
			Return(c, nil, err)

			return
		}

		p, err := s.planGetter.Execute(c.Request.Context(), currentUserID(c), childID)
		if err != nil {
			Return(c, nil, err)

			return
		}

		for _, d := range p.Days {
			if d.Status == domain.PlanDayToday {
				Return(c, toActivityView(d.Activity), nil)

				return
			}
		}

		Return(c, nil, errs.ErrNoPlanDayToday)
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
// @Description - when the activity is the plan day open today, that day is marked done and counts
// @Description   toward the child's streak; plan_day then names it. Any other activity only goes into
// @Description   the history (plan_day null)
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
			Pro:        currentUserPro(c),
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
// @Description - only days of the child's plan count: a day counts when a plan day was done on it;
// @Description   other activities stay in the history but do not keep the streak
// @Description - current: consecutive such days up to today, or up to yesterday while today has none
// @Description   yet (the streak is not lost until a whole day is missed)
// @Description - longest: the longest such run; this_week: such days since Monday; total: plan days done
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

// GetStreakDays godoc swagger
// @Summary a child's streak calendar: done and missed days
// @Description - one entry per day from from to to (both inclusive, YYYY-MM-DD), oldest first, skipping
// @Description   days before the child's first plan started and days after today
// @Description - status: done (a plan day was done), missed (no plan day done that day), today (today,
// @Description   nothing done yet: not missed until the day is over); days are Uzbekistan time (UTC+5)
// @Description - from and to are optional: without them, the last 28 days up to today; at most 366
// @Description   days, and from must not be after to (422)
// @Tags children
// @Security BearerAuth
// @Produce json
// @Param id path string true "child id (UUID)"
// @Param from query string false "first day, YYYY-MM-DD"
// @Param to query string false "last day, YYYY-MM-DD (default today)"
// @Success 200 {object} rest.R{data=[]rest.streakDayView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/children/{id}/streak/days [GET]
func (s *Server) GetStreakDays() gin.HandlerFunc {
	return func(c *gin.Context) {
		childID := c.Param("id")
		if err := ValidateUUID(childID); err != nil {
			Return(c, nil, err)

			return
		}

		var dates [2]time.Time

		for i, name := range []string{"from", "to"} {
			v := c.Query(name)
			if v == "" {
				continue
			}

			d, err := time.Parse(time.DateOnly, v)
			if err != nil {
				Return(c, nil, errs.Errf(errs.ErrValidation, "%s must be a date, YYYY-MM-DD", name))

				return
			}

			dates[i] = d
		}

		days, err := s.streakGetter.Days(c.Request.Context(), currentUserID(c), childID, dates[0], dates[1])
		if err != nil {
			Return(c, nil, err)

			return
		}

		views := make([]streakDayView, 0, len(days))
		for _, d := range days {
			views = append(views, streakDayView{Day: d.Day.Format(time.DateOnly), Status: d.Status})
		}

		Return(c, views, nil)
	}
}

type streakDayView struct {
	Day    string `json:"day" example:"2026-10-12"`
	Status string `json:"status" example:"done" enums:"done,missed,today"`
}

type completionView struct {
	ID          string    `json:"id" example:"5d2f8a90-1c3b-4e6f-8a7d-9b0c1d2e3f4a"`
	ChildID     string    `json:"child_id" example:"3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f"`
	ActivityID  string    `json:"activity_id" example:"7b0c1f1e-2d7a-4d8e-9a55-0f4a0d7f9c11"`
	UserID      *string   `json:"user_id" example:"9a8b7c6d-5e4f-4a3b-2c1d-0e9f8a7b6c5d"`
	CompletedOn string    `json:"completed_on" example:"2026-10-03"`
	Note        *string   `json:"note" example:"Qizil rangni birinchi topdi"`
	CreatedAt   time.Time `json:"created_at"`
	// PlanDay is the plan day (1..7) this completion did; null when it did not count for the plan.
	// Only set on the response of POST /v1/children/{id}/completions.
	PlanDay *int `json:"plan_day" example:"3"`
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
		PlanDay:     c.PlanDay,
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
	Name string `json:"name" example:"Amir"`
	// AgeMonths is the child's age in months: 1..11, then 12..84 in steps of 6, then 96..144
	// in steps of 12.
	AgeMonths *int `json:"age_months" example:"48"`
	// Age is the old field in whole years, read only when age_months is missing.
	Age     *int    `json:"age" example:"4"`
	Gender  string  `json:"gender" example:"male" enums:"male,female"`
	PhotoID *string `json:"photo_id" example:"3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f"`
	// GoalIDs are the goals picked for this child, ids from GET /v1/goals.
	GoalIDs []string `json:"goal_ids" example:"2b6f0cc9-0f3e-4b1a-9a7e-5d8c3e2f1a00"`
}

// CreateChild godoc swagger
// @Summary adds a child to the signed-in user
// @Description - creates the child profile and links it to the caller in the same transaction;
// @Description   the caller is the child's first parent
// @Description - any signed-in role (user, paid_user, admin) may add children
// @Description - name is required, at most 100 characters; gender is male or female
// @Description - age_months is required: 1..11 months, then half years 12..84 (12, 18, 24 ... 84),
// @Description   then whole years 96..144; anything else is 422. The old field age (whole years)
// @Description   is still read when age_months is missing
// @Description - photo_id is an optional id from POST /v1/media (422 if unknown)
// @Description - goal_ids is required: 1 to 20 goal ids from GET /v1/goals, repeats stored once;
// @Description   an id that is not an active goal: 422
// @Description - the child's first 7-day plan is made at once from its goals (GET /v1/children/{id}/plan)
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

		ageMonths := ageMonthsOf(req.AgeMonths, req.Age)
		if ageMonths == nil {
			Return(c, nil, errs.Errf(errs.ErrValidation, "age_months is required"))

			return
		}

		child, err := s.childCreator.Execute(c.Request.Context(), currentUserID(c), currentUserPro(c), domain.Child{
			Name:      req.Name,
			AgeMonths: *ageMonths,
			Gender:    req.Gender,
			PhotoID:   req.PhotoID,
			GoalIDs:   req.GoalIDs,
		})
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toChildView(child), nil)
	}
}

// GetChild godoc swagger
// @Summary one of the signed-in user's children
// @Description - only the child's parents can read it; any other id, or a deleted child, is 404
// @Tags children
// @Security BearerAuth
// @Produce json
// @Param id path string true "child id (UUID)"
// @Success 200 {object} rest.R{data=rest.childView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/children/{id} [GET]
func (s *Server) GetChild() gin.HandlerFunc {
	return func(c *gin.Context) {
		childID := c.Param("id")
		if err := ValidateUUID(childID); err != nil {
			Return(c, nil, err)

			return
		}

		child, err := s.childGetter.Execute(c.Request.Context(), currentUserID(c), childID)
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toChildView(child), nil)
	}
}

type childView struct {
	ID   string `json:"id" example:"3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f"`
	Name string `json:"name" example:"Amir"`
	// AgeMonths is the child's age in months.
	AgeMonths int `json:"age_months" example:"54"`
	// Age is the age in whole years (age_months / 12, rounded down), kept for old clients.
	Age     int     `json:"age" example:"4"`
	Gender  string  `json:"gender" example:"male"`
	PhotoID *string `json:"photo_id" example:"5d2f8a90-1c3b-4e6f-8a7d-9b0c1d2e3f4a"`
	// PhotoURL is where the app loads the photo from (GET, no token); null without a photo.
	PhotoURL *string `json:"photo_url" example:"/v1/media/5d2f8a90-1c3b-4e6f-8a7d-9b0c1d2e3f4a"`
	// photo_main_url, photo_thumb_url, photo_small_url: the sizes of the photo (GET /v1/media/{id}?size=).
	photoURLs
	GoalIDs   []string  `json:"goal_ids" example:"2b6f0cc9-0f3e-4b1a-9a7e-5d8c3e2f1a00"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toChildView(c domain.Child) childView {
	return childView{
		ID:        c.ID,
		Name:      c.Name,
		AgeMonths: c.AgeMonths,
		Age:       c.AgeYears(),
		Gender:    c.Gender,
		PhotoID:   c.PhotoID,
		PhotoURL:  photoURL(c.PhotoID),
		photoURLs: newPhotoURLs(c.PhotoID),
		GoalIDs:   c.GoalIDs,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

// UpdateChildRequest is the body of PATCH /v1/children/{id}. Omitted or null fields are kept;
// "" clears photo_id.
type UpdateChildRequest struct {
	Name *string `json:"name" example:"Amir"`
	// AgeMonths is the child's age in months (same steps as POST /v1/children).
	AgeMonths *int `json:"age_months" example:"60"`
	// Age is the old field in whole years, read only when age_months is missing.
	Age     *int    `json:"age" example:"5"`
	Gender  *string `json:"gender" example:"male" enums:"male,female"`
	PhotoID *string `json:"photo_id" example:"3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f"`
	// GoalIDs, when sent, replaces the child's goals: 1 to 20 ids from GET /v1/goals.
	GoalIDs *[]string `json:"goal_ids" example:"2b6f0cc9-0f3e-4b1a-9a7e-5d8c3e2f1a00"`
}

// UpdateChild godoc swagger
// @Summary edits one of the signed-in user's children
// @Description - only the child's parents can edit it; any other id, or a deleted child, is 404
// @Description - omitted or null fields are kept; the add-child rules apply to the given ones:
// @Description   name 1..100 characters, age_months (or the old age in years), gender male or
// @Description   female, photo_id from POST /v1/media
// @Description   ("" clears it, an unknown id is 422)
// @Description - goal_ids replaces the child's goals (1 to 20 active goal ids, 422 otherwise); the
// @Description   current plan keeps its days, the next plan uses the new goals
// @Description - an empty body is 422 (nothing to update)
// @Tags children
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "child id (UUID)"
// @Param body body UpdateChildRequest true "fields to change"
// @Success 200 {object} rest.R{data=rest.childView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/children/{id} [PATCH]
func (s *Server) UpdateChild() gin.HandlerFunc {
	return func(c *gin.Context) {
		childID := c.Param("id")
		if err := ValidateUUID(childID); err != nil {
			Return(c, nil, err)

			return
		}

		var req UpdateChildRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Return(c, nil, errs.Errf(errs.ErrBadRequest, "invalid JSON body: %s", err.Error()))

			return
		}

		child, err := s.childUpdater.Execute(c.Request.Context(), currentUserID(c), childID, domain.ChildUpdate{
			Name:      req.Name,
			AgeMonths: ageMonthsOf(req.AgeMonths, req.Age),
			Gender:    req.Gender,
			PhotoID:   req.PhotoID,
			GoalIDs:   req.GoalIDs,
		})
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toChildView(child), nil)
	}
}

// GetPlan godoc swagger
// @Summary a child's current 7-day plan (the Reja tab)
// @Description - only the child's parents can ask (404 otherwise)
// @Description - the plan is made when the child is added (or on the first call for older children):
// @Description   seven days, each serving one of the child's goals in turn (goals A, B, C: A B C A B C A,
// @Description   the next plan starting one goal further), with an activity fitting the child's age
// @Description   (clamped to 2..6) that serves the day's goal, preferring activities serving more of the
// @Description   child's goals and ones the child has not done
// @Description - days are done in order, at most one per calendar day (Uzbekistan time): status is done,
// @Description   today (the next day to do, open now) or locked. A missed day pauses the plan: the open
// @Description   day stays open until it is done
// @Description - completing a day is POST /v1/children/{id}/completions with the day's activity id
// @Description - completed is true after the last day; then POST starts the next plan
// @Description - 404 when no published activity fits the child's age
// @Tags children
// @Security BearerAuth
// @Produce json
// @Param id path string true "child id (UUID)"
// @Success 200 {object} rest.R{data=rest.planView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/children/{id}/plan [GET]
func (s *Server) GetPlan() gin.HandlerFunc {
	return s.plan(false)
}

// RestartPlan godoc swagger
// @Summary starts a child's next 7-day plan
// @Description - only after the current plan is done (completed true): 409 otherwise
// @Description - the new plan starts today from the child's goals as they are now; change them first
// @Description   with PATCH /v1/children/{id} if wanted. When the last day of the previous plan was done
// @Description   today, day 1 opens tomorrow
// @Tags children
// @Security BearerAuth
// @Produce json
// @Param id path string true "child id (UUID)"
// @Success 200 {object} rest.R{data=rest.planView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 409 {object} rest.ConflictResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/children/{id}/plan [POST]
func (s *Server) RestartPlan() gin.HandlerFunc {
	return s.plan(true)
}

func (s *Server) plan(next bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		childID := c.Param("id")
		if err := ValidateUUID(childID); err != nil {
			Return(c, nil, err)

			return
		}

		get := s.planGetter.Execute
		if next {
			get = s.planGetter.Next
		}

		p, err := get(c.Request.Context(), currentUserID(c), childID)
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toPlanView(p), nil)
	}
}

type planView struct {
	ID        string        `json:"id" example:"8c1e2d3f-4a5b-4c6d-8e7f-9a0b1c2d3e4f"`
	ChildID   string        `json:"child_id" example:"3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f"`
	Number    int           `json:"number" example:"1"` // the child's plans counted from 1
	StartedOn string        `json:"started_on" example:"2026-10-11"`
	Completed bool          `json:"completed" example:"false"`
	Days      []planDayView `json:"days"`
}

type planDayView struct {
	Day         int          `json:"day" example:"1"`
	GoalID      *string      `json:"goal_id" example:"2b6f0cc9-0f3e-4b1a-9a7e-5d8c3e2f1a00"` // null when the child had no goals
	Status      string       `json:"status" example:"today" enums:"done,today,locked"`
	CompletedOn *string      `json:"completed_on" example:"2026-10-11"`
	Activity    activityView `json:"activity"`
}

func toPlanView(p domain.Plan) planView {
	v := planView{
		ID:        p.ID,
		ChildID:   p.ChildID,
		Number:    p.Number,
		StartedOn: p.StartedOn.Format(time.DateOnly),
		Completed: p.Completed,
		Days:      make([]planDayView, 0, len(p.Days)),
	}

	for _, d := range p.Days {
		dv := planDayView{
			Day:      d.Day,
			GoalID:   d.GoalID,
			Status:   d.Status,
			Activity: toActivityView(d.Activity),
		}

		if d.CompletedOn != nil {
			on := d.CompletedOn.Format(time.DateOnly)
			dv.CompletedOn = &on
		}

		v.Days = append(v.Days, dv)
	}

	return v
}

const monthsPerYear = 12

// ageMonthsOf picks age_months, or the old age field in years turned into months.
func ageMonthsOf(months, years *int) *int {
	if months != nil {
		return months
	}

	if years != nil {
		m := *years * monthsPerYear

		return &m
	}

	return nil
}
