package rest

import (
	"github.com/gin-gonic/gin"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

// UpdateProfileRequest is the body of PATCH /v1/me. Omitted or null fields are left unchanged;
// "" clears name, username or photo_id.
type UpdateProfileRequest struct {
	Name        *string `json:"name" example:"Dilnoza"`
	Username    *string `json:"username" example:"dilnoza_95"`
	PhoneNumber *string `json:"phone_number" example:"+998901234567"`
	PhotoID     *string `json:"photo_id" example:"3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f"`
}

// ResetPasswordRequest is the body of PUT /v1/me/password.
type ResetPasswordRequest struct {
	Password string `json:"password" example:"n3w-s3cret-pass"`
}

type profileView struct {
	userView
	Role domain.UserRole `json:"role" example:"user" enums:"user,paid_user,admin"`
}

// GetProfile godoc swagger
// @Summary the signed-in user's profile
// @Description - the user row plus the current role (user, paid_user or admin)
// @Tags me
// @Security BearerAuth
// @Produce json
// @Success 200 {object} rest.R{data=rest.profileView}
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/me [GET]
func (s *Server) GetProfile() gin.HandlerFunc {
	return func(c *gin.Context) {
		u, err := s.userGetter.Execute(c.Request.Context(), currentUserID(c))
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, profileView{userView: toUserView(u), Role: currentUserRole(c)}, nil)
	}
}

// UpdateProfile godoc swagger
// @Summary edits the signed-in user's profile partially
// @Description - only the fields present in the body change; omitted or null fields are kept
// @Description - "" clears name, username or photo_id; phone_number cannot be cleared
// @Description - name: at most 100 characters; username: 3 to 32 of a-z, 0-9, '_' or '.' (lowercased);
// @Description   photo_id: an id from POST /v1/media (422 if unknown)
// @Description - a new phone_number must be an Uzbek number (+998 and 9 digits) verified first:
// @Description   POST /v1/otp/send and /v1/otp/verify with purpose update_user for the new number (403 otherwise);
// @Description   the change uses the verification up. Sending the current number is not a change
// @Description - username or phone number already used by someone else: 409
// @Tags me
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body UpdateProfileRequest true "fields to change"
// @Success 200 {object} rest.R{data=rest.profileView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 403 {object} rest.ForbiddenResponse
// @Failure 409 {object} rest.ConflictResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/me [PATCH]
func (s *Server) UpdateProfile() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req UpdateProfileRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Return(c, nil, errs.Errf(errs.ErrBadRequest, "invalid JSON body: %s", err.Error()))

			return
		}

		u, err := s.profileUpdater.Execute(c.Request.Context(), currentUserID(c), domain.UserUpdate{
			Name:        req.Name,
			Username:    req.Username,
			PhoneNumber: req.PhoneNumber,
			PhotoID:     req.PhotoID,
		})
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, profileView{userView: toUserView(u), Role: currentUserRole(c)}, nil)
	}
}

// ResetPassword godoc swagger
// @Summary sets a new password for the signed-in user, confirmed by an SMS code
// @Description - first POST /v1/otp/send and /v1/otp/verify with purpose reset_password for the
// @Description   user's own phone number; without that verification: 403. A successful reset uses it up
// @Description - password: 8 to 72 bytes, stored as a bcrypt hash (422)
// @Description - returns a new token pair; every token issued before, on any device, stops working
// @Tags me
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body ResetPasswordRequest true "new password"
// @Success 200 {object} rest.R{data=rest.tokenPairView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 403 {object} rest.ForbiddenResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/me/password [PUT]
func (s *Server) ResetPassword() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ResetPasswordRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Return(c, nil, errs.Errf(errs.ErrBadRequest, "invalid JSON body: %s", err.Error()))

			return
		}

		pair, err := s.passwordResetter.Execute(c.Request.Context(), currentUserID(c), req.Password)
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toTokenPairView(pair), nil)
	}
}

// DeleteAccount godoc swagger
// @Summary deletes the signed-in user's account
// @Description - a soft delete: the user disappears from reads and their username and phone number can be used again
// @Description - their tokens are removed at once, so the access token used here stops working
// @Description - their children go too, unless another parent still has the child
// @Tags me
// @Security BearerAuth
// @Produce json
// @Success 200 {object} rest.R
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/me [DELETE]
func (s *Server) DeleteAccount() gin.HandlerFunc {
	return func(c *gin.Context) {
		Return(c, nil, s.userDeleter.Execute(c.Request.Context(), currentUserID(c)))
	}
}

// ListMyChildren godoc swagger
// @Summary lists the signed-in user's children, newest first
// @Description - only children linked to the caller; deleted children are left out
// @Description - GET /v1/children/{id} returns one of them
// @Tags me
// @Security BearerAuth
// @Produce json
// @Param limit query int false "page size (default 20, max 100)"
// @Param offset query int false "offset"
// @Success 200 {object} rest.R{data=rest.childListView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/me/children [GET]
func (s *Server) ListMyChildren() gin.HandlerFunc {
	return func(c *gin.Context) {
		limit, offset, err := parsePagination(c)
		if err != nil {
			Return(c, nil, err)

			return
		}

		items, total, err := s.childLister.Execute(c.Request.Context(), currentUserID(c),
			domain.ChildFilter{Limit: limit, Offset: offset})
		if err != nil {
			Return(c, nil, err)

			return
		}

		views := make([]childView, 0, len(items))
		for i := range items {
			views = append(views, toChildView(items[i]))
		}

		Return(c, childListView{Items: views, Total: total, Limit: limit, Offset: offset}, nil)
	}
}

type childListView struct {
	Items  []childView `json:"items"`
	Total  int         `json:"total" example:"2"`
	Limit  int         `json:"limit" example:"20"`
	Offset int         `json:"offset" example:"0"`
}
