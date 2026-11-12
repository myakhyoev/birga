package rest

import (
	"time"

	"github.com/gin-gonic/gin"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

// MilestoneRequest is the body of POST /v1/admin/milestones and PATCH /v1/admin/milestones/{id}.
// On PATCH, omitted or null fields are left unchanged.
type MilestoneRequest struct {
	Domain       *string  `json:"domain" example:"communication" enums:"communication,social,thinking,movement"`
	AgeMonths    *int     `json:"age_months" example:"48" minimum:"2" maximum:"72"`
	SortOrder    *int     `json:"sort_order" example:"1"`
	TextUz       *string  `json:"text_uz" example:"4 va undan ko‘p so‘zli gaplar tuzadi"`
	TextRu       *string  `json:"text_ru" example:"Говорит предложениями из 4 и более слов"`
	TextEn       *string  `json:"text_en" example:"Says sentences with four or more words"`
	MeaningUz    *string  `json:"meaning_uz"`
	MeaningRu    *string  `json:"meaning_ru"`
	MeaningEn    *string  `json:"meaning_en"`
	HowToUz      *string  `json:"how_to_uz"`
	HowToRu      *string  `json:"how_to_ru"`
	HowToEn      *string  `json:"how_to_en"`
	SpecialistUz *string  `json:"specialist_uz"`
	SpecialistRu *string  `json:"specialist_ru"`
	SpecialistEn *string  `json:"specialist_en"`
	ActivityIDs  []string `json:"activity_ids" example:"7b0c1f1e-2d7a-4d8e-9a55-0f4a0d7f9c11"`
	Status       *string  `json:"status" example:"in_review" enums:"published,in_review,unpublished"`
}

func (r MilestoneRequest) update() domain.MilestoneUpdate {
	return domain.MilestoneUpdate{
		Domain: r.Domain, AgeMonths: r.AgeMonths, SortOrder: r.SortOrder,
		TextUz: r.TextUz, TextRu: r.TextRu, TextEn: r.TextEn,
		MeaningUz: r.MeaningUz, MeaningRu: r.MeaningRu, MeaningEn: r.MeaningEn,
		HowToUz: r.HowToUz, HowToRu: r.HowToRu, HowToEn: r.HowToEn,
		SpecialistUz: r.SpecialistUz, SpecialistRu: r.SpecialistRu, SpecialistEn: r.SpecialistEn,
		ActivityIDs: r.ActivityIDs, Status: r.Status,
	}
}

func (r MilestoneRequest) milestone() domain.Milestone {
	m := domain.Milestone{
		Domain: deref(r.Domain), TextUz: deref(r.TextUz), TextRu: deref(r.TextRu), TextEn: deref(r.TextEn),
		MeaningUz: deref(r.MeaningUz), MeaningRu: deref(r.MeaningRu), MeaningEn: deref(r.MeaningEn),
		HowToUz: deref(r.HowToUz), HowToRu: deref(r.HowToRu), HowToEn: deref(r.HowToEn),
		SpecialistUz: deref(r.SpecialistUz), SpecialistRu: deref(r.SpecialistRu), SpecialistEn: deref(r.SpecialistEn),
		ActivityIDs: r.ActivityIDs, Status: deref(r.Status),
	}

	if r.AgeMonths != nil {
		m.AgeMonths = *r.AgeMonths
	}

	if r.SortOrder != nil {
		m.SortOrder = *r.SortOrder
	}

	return m
}

// CreateMilestone godoc swagger
// @Summary creates a development milestone (skill)
// @Description - domain is communication, social, thinking or movement; age_months is the stage, 2..72
// @Description - text_uz, text_ru and text_en are required, at most 300 characters each
// @Description - the advice texts (meaning_*, how_to_* one step per line, specialist_*) are optional, at most 3000 characters
// @Description - activity_ids: up to 10 activities ("Mos mashg‘ulotlar"); unknown ids are 422
// @Description - status is published, in_review or unpublished; omitted means in_review
// @Tags admin
// @Security AdminKey
// @Accept json
// @Produce json
// @Param body body MilestoneRequest true "milestone"
// @Success 200 {object} rest.R{data=rest.milestoneView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/admin/milestones [POST]
func (s *Server) CreateMilestone() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req MilestoneRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Return(c, nil, errs.Errf(errs.ErrBadRequest, "invalid JSON body: %s", err.Error()))

			return
		}

		m, err := s.milestones.Create(c.Request.Context(), req.milestone())
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toMilestoneView(m), nil)
	}
}

// AdminListMilestones godoc swagger
// @Summary lists milestones in every status
// @Description Ordered by stage, then domain (communication, social, thinking, movement), then sort_order.
// @Tags admin
// @Security AdminKey
// @Produce json
// @Param q query string false "text the milestone (any language) contains, ignoring case (at most 100 characters)"
// @Param domain query string false "communication, social, thinking or movement"
// @Param age_months query int false "stage, 2..72"
// @Param status query string false "published, in_review or unpublished; omitted lists all" Enums(published, in_review, unpublished)
// @Param limit query int false "page size (default 20, max 100)"
// @Param offset query int false "offset"
// @Success 200 {object} rest.R{data=rest.milestoneListView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/admin/milestones [GET]
func (s *Server) AdminListMilestones() gin.HandlerFunc {
	return func(c *gin.Context) {
		af, ok := articleFilter(c)
		if !ok {
			return
		}

		ageMonths, err := parseOptionalInt(c, "age_months")
		if err != nil {
			Return(c, nil, err)

			return
		}

		f := domain.MilestoneFilter{
			Domain: c.Query("domain"), AgeMonths: ageMonths, Status: c.Query("status"), Search: af.Search,
			Limit: af.Limit, Offset: af.Offset,
		}

		items, total, err := s.milestones.List(c.Request.Context(), f)
		if err != nil {
			Return(c, nil, err)

			return
		}

		views := make([]milestoneView, 0, len(items))
		for i := range items {
			views = append(views, toMilestoneView(items[i]))
		}

		Return(c, milestoneListView{Items: views, Total: total, Limit: f.Limit, Offset: f.Offset}, nil)
	}
}

// AdminGetMilestone godoc swagger
// @Summary returns a milestone in any status
// @Tags admin
// @Security AdminKey
// @Produce json
// @Param id path string true "milestone id (UUID)"
// @Success 200 {object} rest.R{data=rest.milestoneView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/admin/milestones/{id} [GET]
func (s *Server) AdminGetMilestone() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := ValidateUUID(id); err != nil {
			Return(c, nil, err)

			return
		}

		m, err := s.milestones.Get(c.Request.Context(), id)
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toMilestoneView(m), nil)
	}
}

// UpdateMilestone godoc swagger
// @Summary changes some fields of a milestone
// @Description Each given field follows the create rules; the required texts cannot be emptied, the advice texts can.
// @Description activity_ids, when sent, replaces the list.
// @Tags admin
// @Security AdminKey
// @Accept json
// @Produce json
// @Param id path string true "milestone id (UUID)"
// @Param body body MilestoneRequest true "fields to change"
// @Success 200 {object} rest.R{data=rest.milestoneView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/admin/milestones/{id} [PATCH]
func (s *Server) UpdateMilestone() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := ValidateUUID(id); err != nil {
			Return(c, nil, err)

			return
		}

		var req MilestoneRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Return(c, nil, errs.Errf(errs.ErrBadRequest, "invalid JSON body: %s", err.Error()))

			return
		}

		m, err := s.milestones.Update(c.Request.Context(), id, req.update())
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toMilestoneView(m), nil)
	}
}

// DeleteMilestone godoc swagger
// @Summary soft-deletes a milestone
// @Description It leaves every list; children's ticks of it are kept but no longer shown or counted.
// @Tags admin
// @Security AdminKey
// @Produce json
// @Param id path string true "milestone id (UUID)"
// @Success 200 {object} rest.R
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/admin/milestones/{id} [DELETE]
func (s *Server) DeleteMilestone() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := ValidateUUID(id); err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, nil, s.milestones.Delete(c.Request.Context(), id))
	}
}

// GetChildMilestones godoc swagger
// @Summary returns a child's milestones of one stage ("Ko‘nikmalar")
// @Description - age_months picks the stage; omitted means the child's stage, the highest stage at or below the child's age
// @Description - stages lists the stages that have published milestones, for the stage picker
// @Description - domains come in the order communication, social, thinking, movement; each milestone says achieved
// @Description - without Birga Pro the advice fields are empty (has_advice still tells whether advice exists)
// @Tags children
// @Security BearerAuth
// @Produce json
// @Param id path string true "child id (UUID)"
// @Param age_months query int false "stage, 2..72"
// @Success 200 {object} rest.R{data=rest.childMilestonesView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/children/{id}/milestones [GET]
func (s *Server) GetChildMilestones() gin.HandlerFunc {
	return func(c *gin.Context) {
		childID := c.Param("id")
		if err := ValidateUUID(childID); err != nil {
			Return(c, nil, err)

			return
		}

		ageMonths, err := parseOptionalInt(c, "age_months")
		if err != nil {
			Return(c, nil, err)

			return
		}

		m, err := s.childMilestones.List(c.Request.Context(), currentUserID(c), currentUserPro(c), childID, ageMonths)
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toChildMilestonesView(m), nil)
	}
}

// MarkMilestone godoc swagger
// @Summary marks a milestone achieved for the child ("O‘zlashtirdi")
// @Description Any parent of the child may mark, on any plan. Marking again keeps the first time.
// @Description 404 for an unknown or unpublished milestone.
// @Tags children
// @Security BearerAuth
// @Produce json
// @Param id path string true "child id (UUID)"
// @Param milestone_id path string true "milestone id (UUID)"
// @Success 200 {object} rest.R
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/children/{id}/milestones/{milestone_id} [PUT]
func (s *Server) MarkMilestone() gin.HandlerFunc {
	return func(c *gin.Context) {
		childID := c.Param("id")
		if err := ValidateUUID(childID); err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, nil, s.childMilestones.Mark(c.Request.Context(), currentUserID(c), childID, c.Param("milestone_id")))
	}
}

// UnmarkMilestone godoc swagger
// @Summary removes the child's mark of a milestone
// @Tags children
// @Security BearerAuth
// @Produce json
// @Param id path string true "child id (UUID)"
// @Param milestone_id path string true "milestone id (UUID)"
// @Success 200 {object} rest.R
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/children/{id}/milestones/{milestone_id} [DELETE]
func (s *Server) UnmarkMilestone() gin.HandlerFunc {
	return func(c *gin.Context) {
		childID := c.Param("id")
		if err := ValidateUUID(childID); err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, nil, s.childMilestones.Unmark(c.Request.Context(), currentUserID(c), childID, c.Param("milestone_id")))
	}
}

// GetMilestone godoc swagger
// @Summary returns one published milestone with its advice ("Maslahat")
// @Description With child_id (one of the caller's children) it says whether the child achieved it. activities are the
// @Description related published activities. Without Birga Pro the advice fields are empty and activities is [].
// @Tags milestones
// @Security BearerAuth
// @Produce json
// @Param id path string true "milestone id (UUID)"
// @Param child_id query string false "one of the caller's children"
// @Success 200 {object} rest.R{data=rest.milestoneDetailView}
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/milestones/{id} [GET]
func (s *Server) GetMilestone() gin.HandlerFunc {
	return func(c *gin.Context) {
		d, err := s.childMilestones.Get(c.Request.Context(), currentUserID(c), currentUserPro(c), c.Param("id"), c.Query("child_id"))
		if err != nil {
			Return(c, nil, err)

			return
		}

		v := milestoneDetailView{
			childMilestoneView: toChildMilestoneView(d.ChildMilestone),
			Activities:         make([]activityView, 0, len(d.Activities)),
		}
		for i := range d.Activities {
			v.Activities = append(v.Activities, toActivityView(d.Activities[i]))
		}

		Return(c, v, nil)
	}
}

// GetMonthlyReport godoc swagger
// @Summary returns a child's monthly report (Birga Pro)
// @Description new_milestones counts the milestones marked in the month, completions the activities done (Uzbekistan time).
// @Tags children
// @Security BearerAuth
// @Produce json
// @Param id path string true "child id (UUID)"
// @Param month query string false "YYYY-MM; omitted means this month"
// @Success 200 {object} rest.R{data=rest.monthlyReportView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 403 {object} rest.ForbiddenResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/children/{id}/report [GET]
func (s *Server) GetMonthlyReport() gin.HandlerFunc {
	return func(c *gin.Context) {
		childID := c.Param("id")
		if err := ValidateUUID(childID); err != nil {
			Return(c, nil, err)

			return
		}

		r, err := s.childMilestones.Report(c.Request.Context(), currentUserID(c), currentUserPro(c), childID, c.Query("month"))
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, monthlyReportView{Month: r.Month.Format("2006-01"), NewMilestones: r.NewMilestones, Completions: r.Completions}, nil)
	}
}

type milestoneView struct {
	ID           string    `json:"id" example:"5d1c9e2a-7b3f-4a6e-9c0d-2e8f1a7b4c3d"`
	Domain       string    `json:"domain" example:"communication"`
	AgeMonths    int       `json:"age_months" example:"48"`
	SortOrder    int       `json:"sort_order" example:"1"`
	TextUz       string    `json:"text_uz" example:"4 va undan ko‘p so‘zli gaplar tuzadi"`
	TextRu       string    `json:"text_ru" example:"Говорит предложениями из 4 и более слов"`
	TextEn       string    `json:"text_en" example:"Says sentences with four or more words"`
	MeaningUz    string    `json:"meaning_uz"`
	MeaningRu    string    `json:"meaning_ru"`
	MeaningEn    string    `json:"meaning_en"`
	HowToUz      string    `json:"how_to_uz"`
	HowToRu      string    `json:"how_to_ru"`
	HowToEn      string    `json:"how_to_en"`
	SpecialistUz string    `json:"specialist_uz"`
	SpecialistRu string    `json:"specialist_ru"`
	SpecialistEn string    `json:"specialist_en"`
	ActivityIDs  []string  `json:"activity_ids"`
	HasAdvice    bool      `json:"has_advice" example:"true"`
	AdviceLocked bool      `json:"advice_locked" example:"false"`
	Status       string    `json:"status" example:"published"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type milestoneListView struct {
	Items  []milestoneView `json:"items"`
	Total  int             `json:"total" example:"57"`
	Limit  int             `json:"limit" example:"20"`
	Offset int             `json:"offset" example:"0"`
}

type childMilestoneView struct {
	milestoneView
	Achieved   bool       `json:"achieved" example:"true"`
	AchievedAt *time.Time `json:"achieved_at"`
}

type milestoneDomainView struct {
	Domain     string               `json:"domain" example:"communication"`
	Milestones []childMilestoneView `json:"milestones"`
}

type childMilestonesView struct {
	AgeMonths int                   `json:"age_months" example:"48"`
	Stages    []int                 `json:"stages" example:"24,30,36,48,60"`
	Achieved  int                   `json:"achieved" example:"5"`
	Total     int                   `json:"total" example:"13"`
	Domains   []milestoneDomainView `json:"domains"`
}

type milestoneDetailView struct {
	childMilestoneView
	Activities []activityView `json:"activities"`
}

type monthlyReportView struct {
	Month         string `json:"month" example:"2026-10"`
	NewMilestones int    `json:"new_milestones" example:"3"`
	Completions   int    `json:"completions" example:"26"`
}

func toMilestoneView(m domain.Milestone) milestoneView {
	return milestoneView{
		ID: m.ID, Domain: m.Domain, AgeMonths: m.AgeMonths, SortOrder: m.SortOrder,
		TextUz: m.TextUz, TextRu: m.TextRu, TextEn: m.TextEn,
		MeaningUz: m.MeaningUz, MeaningRu: m.MeaningRu, MeaningEn: m.MeaningEn,
		HowToUz: m.HowToUz, HowToRu: m.HowToRu, HowToEn: m.HowToEn,
		SpecialistUz: m.SpecialistUz, SpecialistRu: m.SpecialistRu, SpecialistEn: m.SpecialistEn,
		ActivityIDs: m.ActivityIDs, HasAdvice: m.HasAdvice(), AdviceLocked: m.AdviceLocked, Status: m.Status,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func toChildMilestoneView(m domain.ChildMilestone) childMilestoneView {
	return childMilestoneView{milestoneView: toMilestoneView(m.Milestone), Achieved: m.Achieved, AchievedAt: m.AchievedAt}
}

func toChildMilestonesView(m domain.ChildMilestones) childMilestonesView {
	v := childMilestonesView{AgeMonths: m.AgeMonths, Stages: m.Stages, Achieved: m.Achieved, Total: m.Total,
		Domains: make([]milestoneDomainView, 0, len(domain.MilestoneDomains))}
	if v.Stages == nil {
		v.Stages = []int{}
	}

	for _, d := range domain.MilestoneDomains {
		dv := milestoneDomainView{Domain: d, Milestones: make([]childMilestoneView, 0, len(m.Domains[d]))}
		for _, it := range m.Domains[d] {
			dv.Milestones = append(dv.Milestones, toChildMilestoneView(it))
		}

		v.Domains = append(v.Domains, dv)
	}

	return v
}
