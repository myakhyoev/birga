package rest

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

// CreateUserRequest is the body of POST /v1/admin/users.
type CreateUserRequest struct {
	Name        *string `json:"name" example:"Dilnoza"`
	Username    *string `json:"username" example:"dilnoza_95"`
	PhoneNumber *string `json:"phone_number" example:"+998901234567"`
	PhotoID     *string `json:"photo_id" example:"3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f"`
}

// UpdateUserRequest is the body of PATCH /v1/admin/users/{id}. Omitted or null
// fields are left unchanged; "" clears name, username or photo_id.
type UpdateUserRequest struct {
	Name        *string `json:"name" example:"Dilnoza"`
	Username    *string `json:"username" example:"dilnoza_95"`
	PhoneNumber *string `json:"phone_number" example:"+998901234567"`
	PhotoID     *string `json:"photo_id" example:"3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f"`
}

// CreateUser godoc swagger
// @Summary creates a user
// @Description - phone_number is required, in E.164 format (+998901234567)
// @Description - username is optional: 3..32 latin letters, digits, '_' or '.'; stored lowercase
// @Description - name is optional, at most 100 characters; photo_id is an optional id from POST /v1/media (422 if unknown)
// @Description - username and phone_number must be unique among non-deleted users (409)
// @Tags admin
// @Security AdminKey
// @Accept json
// @Produce json
// @Param body body CreateUserRequest true "user"
// @Success 200 {object} rest.R{data=rest.userView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 409 {object} rest.ConflictResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/admin/users [POST]
func (s *Server) CreateUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreateUserRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Return(c, nil, errs.Errf(errs.ErrBadRequest, "invalid JSON body: %s", err.Error()))

			return
		}

		u, err := s.userCreator.Execute(c.Request.Context(), domain.User{
			Name:        req.Name,
			Username:    req.Username,
			PhoneNumber: req.PhoneNumber,
			PhotoID:     req.PhotoID,
		})
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toUserView(u), nil)
	}
}

// ListUsers godoc swagger
// @Summary lists users (soft-deleted users are excluded)
// @Description - q finds users: username starts with it, or name / phone number contains it
// @Description (case-insensitive, a leading @ is ignored); an exact username match comes first
// @Tags admin
// @Security AdminKey
// @Produce json
// @Param q query string false "search text (username, name or phone number)"
// @Param limit query int false "page size (default 20, max 100)"
// @Param offset query int false "offset"
// @Success 200 {object} rest.R{data=rest.userListView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/admin/users [GET]
func (s *Server) ListUsers() gin.HandlerFunc {
	return func(c *gin.Context) {
		limit, offset, err := parsePagination(c)
		if err != nil {
			Return(c, nil, err)

			return
		}

		items, total, err := s.userLister.Execute(c.Request.Context(), domain.UserFilter{
			Limit: limit, Offset: offset, Query: strings.TrimPrefix(strings.TrimSpace(c.Query("q")), "@"),
		})
		if err != nil {
			Return(c, nil, err)

			return
		}

		views := make([]userView, 0, len(items))
		for i := range items {
			views = append(views, toUserView(items[i]))
		}

		Return(c, userListView{Items: views, Total: total, Limit: limit, Offset: offset}, nil)
	}
}

// GetUser godoc swagger
// @Summary returns a user
// @Tags admin
// @Security AdminKey
// @Produce json
// @Param id path string true "user id (UUID)"
// @Success 200 {object} rest.R{data=rest.userView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/admin/users/{id} [GET]
func (s *Server) GetUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := ValidateUUID(id); err != nil {
			Return(c, nil, err)

			return
		}

		u, err := s.userGetter.Execute(c.Request.Context(), id)
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toUserView(u), nil)
	}
}

// UpdateUser godoc swagger
// @Summary updates a user partially
// @Description - only the fields present in the body change; omitted or null fields are kept
// @Description - "" clears name, username or photo_id; phone_number cannot be cleared
// @Description - same format and uniqueness rules as create
// @Tags admin
// @Security AdminKey
// @Accept json
// @Produce json
// @Param id path string true "user id (UUID)"
// @Param body body UpdateUserRequest true "fields to change"
// @Success 200 {object} rest.R{data=rest.userView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 409 {object} rest.ConflictResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/admin/users/{id} [PATCH]
func (s *Server) UpdateUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := ValidateUUID(id); err != nil {
			Return(c, nil, err)

			return
		}

		var req UpdateUserRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Return(c, nil, errs.Errf(errs.ErrBadRequest, "invalid JSON body: %s", err.Error()))

			return
		}

		u, err := s.userUpdater.Execute(c.Request.Context(), id, domain.UserUpdate{
			Name:        req.Name,
			Username:    req.Username,
			PhoneNumber: req.PhoneNumber,
			PhotoID:     req.PhotoID,
		})
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toUserView(u), nil)
	}
}

// DeleteUser godoc swagger
// @Summary soft-deletes a user
// @Description Sets deleted_at; the user disappears from reads, their tokens are removed,
// @Description and their username and phone number can be used again.
// @Tags admin
// @Security AdminKey
// @Produce json
// @Param id path string true "user id (UUID)"
// @Success 200 {object} rest.R
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/admin/users/{id} [DELETE]
func (s *Server) DeleteUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := ValidateUUID(id); err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, nil, s.userDeleter.Execute(c.Request.Context(), id))
	}
}

type userView struct {
	ID          string  `json:"id" example:"7b0c1f1e-2d7a-4d8e-9a55-0f4a0d7f9c11"`
	Name        *string `json:"name" example:"Dilnoza"`
	Username    *string `json:"username" example:"dilnoza_95"`
	PhoneNumber *string `json:"phone_number" example:"+998901234567"`
	PhotoID     *string `json:"photo_id" example:"3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f"`
	// PhotoURL is where clients load the photo from (GET, no token); null without a photo.
	PhotoURL *string `json:"photo_url" example:"/v1/media/3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f"`
	// photo_main_url, photo_thumb_url, photo_small_url: the sizes of the photo (GET /v1/media/{id}?size=).
	photoURLs
	// Relationship is null for users created before sign-up asked for it, or by an admin.
	Relationship *string `json:"relationship" example:"mother" enums:"father,mother,educator,nanny"`
	// AgreedOnConditions is the consent to the terms given at sign-up.
	AgreedOnConditions bool `json:"agreed_on_conditions" example:"true"`
	// ReminderEnabled and ReminderTime are the daily reminder setting (HH:MM, Uzbekistan time).
	ReminderEnabled bool   `json:"reminder_enabled" example:"true"`
	ReminderTime    string `json:"reminder_time" example:"18:00"`
	// Role is the sign-in role; null for a user without a sign-in account (created by an admin).
	Role *string `json:"role" example:"user" enums:"unverified_user,user,paid_user,admin"`
	// Plan is "pro" while the user has Birga Pro, otherwise "free": roles paid_user and admin, or
	// pro_until in the future.
	Plan string `json:"plan" example:"free" enums:"free,pro"`
	// ProUntil is when Birga Pro given by date ends; null when it was never given.
	ProUntil  *time.Time `json:"pro_until"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type userListView struct {
	Items  []userView `json:"items"`
	Total  int        `json:"total" example:"42"`
	Limit  int        `json:"limit" example:"20"`
	Offset int        `json:"offset" example:"0"`
}

func toUserView(u domain.User) userView {
	return userView{
		ID:           u.ID,
		Name:         u.Name,
		Username:     u.Username,
		PhoneNumber:  u.PhoneNumber,
		PhotoID:      u.PhotoID,
		PhotoURL:     photoURL(u.PhotoID),
		photoURLs:    newPhotoURLs(u.PhotoID),
		Relationship: u.Relationship,

		AgreedOnConditions: u.AgreedOnConditions,
		ReminderEnabled:    u.ReminderEnabled,
		ReminderTime:       u.ReminderTime,
		Role:               roleView(u.Role),
		Plan:               planName(domain.IsPro(u.Role, u.ProUntil, time.Now())),
		ProUntil:           u.ProUntil,
		CreatedAt:          u.CreatedAt,
		UpdatedAt:          u.UpdatedAt,
	}
}

func roleView(r domain.UserRole) *string {
	if r == "" {
		return nil
	}

	s := string(r)

	return &s
}

func planName(pro bool) string {
	if pro {
		return domain.PlanPro
	}

	return domain.PlanFree
}
