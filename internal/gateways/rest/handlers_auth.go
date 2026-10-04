package rest

import (
	"github.com/gin-gonic/gin"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

// SignUpRequest is the body of POST /v1/auth/signup.
type SignUpRequest struct {
	Name        string `json:"name" example:"Dilnoza"`
	Username    string `json:"username" example:"dilnoza_k"`
	Password    string `json:"password" example:"s3cret-pass"`
	PhoneNumber string `json:"phone_number" example:"+998901234567"`
	// UserRole is optional and defaults to user; admin is refused with 403.
	UserRole string `json:"user_role" enums:"user,paid_user" example:"user"`
}

type tokenPairView struct {
	AccessToken  string `json:"access_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	RefreshToken string `json:"refresh_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	ExpiresIn    int    `json:"expires_in" example:"86400"` // seconds until the access token expires
}

// SignUp godoc swagger
// @Summary creates a user and returns their tokens
// @Description - the phone number must be verified first: POST /v1/otp/send and /v1/otp/verify with
// @Description   purpose sign_up; the verification lasts OTP_VERIFIED_TTL (default 10 minutes) and is
// @Description   used up by a successful sign-up. Not verified: 403
// @Description - name: 1 to 100 characters; username: 3 to 32 of a-z, 0-9, '_' or '.' (lowercased);
// @Description   password: 8 to 72 bytes, stored as a bcrypt hash; phone_number: +998 and 9 digits (422)
// @Description - user_role: user (default when omitted) or paid_user; admin cannot be chosen here (403),
// @Description   anything else is 422. Stored in user_auth.role and written into the tokens as the role claim
// @Description - username or phone number already used: 409
// @Description - access_token (JWT_ACCESS_TTL, default 24 hours) and refresh_token (JWT_REFRESH_TTL,
// @Description   default 0: never expires) are HS256 JWTs; send the access token as Authorization: Bearer <token>
// @Tags auth
// @Accept json
// @Produce json
// @Param body body SignUpRequest true "new user"
// @Success 200 {object} rest.R{data=rest.tokenPairView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 403 {object} rest.ForbiddenResponse
// @Failure 409 {object} rest.ConflictResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/auth/signup [POST]
func (s *Server) SignUp() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req SignUpRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Return(c, nil, errs.Errf(errs.ErrBadRequest, "invalid JSON body: %s", err.Error()))

			return
		}

		pair, err := s.signUp.Execute(c.Request.Context(), domain.SignUpRequest{
			Name:        req.Name,
			Username:    req.Username,
			Password:    req.Password,
			PhoneNumber: req.PhoneNumber,
			Role:        domain.UserRole(req.UserRole),
		})
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toTokenPairView(pair), nil)
	}
}

func toTokenPairView(pair domain.TokenPair) tokenPairView {
	return tokenPairView{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		ExpiresIn:    int(pair.AccessExpiresIn.Seconds()),
	}
}

// RefreshTokenRequest is the body of POST /v1/auth/refresh.
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
}

type accessTokenView struct {
	AccessToken string `json:"access_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	ExpiresIn   int    `json:"expires_in" example:"86400"` // seconds until the access token expires
}

// RefreshToken godoc swagger
// @Summary trades a refresh token for a new access token
// @Description - refresh_token must be signed by this API, unexpired, of type refresh, and still the
// @Description   one stored for the user (a deleted user's token stops working); otherwise 401
// @Description - the refresh token is not rotated; keep using it until it expires
// @Tags auth
// @Accept json
// @Produce json
// @Param body body RefreshTokenRequest true "refresh token from sign-up"
// @Success 200 {object} rest.R{data=rest.accessTokenView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/auth/refresh [POST]
func (s *Server) RefreshToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req RefreshTokenRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Return(c, nil, errs.Errf(errs.ErrBadRequest, "invalid JSON body: %s", err.Error()))

			return
		}

		access, err := s.tokenRefresher.Execute(c.Request.Context(), req.RefreshToken)
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, accessTokenView{AccessToken: access.Token, ExpiresIn: int(access.ExpiresIn.Seconds())}, nil)
	}
}

// LoginRequest is the body of POST /v1/auth/login.
type LoginRequest struct {
	Username string `json:"username" example:"dilnoza_k"`
	Password string `json:"password" example:"s3cret-pass"`
}

// Login godoc swagger
// @Summary signs a user in with username and password
// @Description - username is case-insensitive; username and password are required (422)
// @Description - unknown username or wrong password: 401 with the same message, so it does not tell
// @Description   which usernames exist
// @Description - returns a new token pair, like sign-up. Only the newest pair is valid, so signing in
// @Description   signs out every other device
// @Tags auth
// @Accept json
// @Produce json
// @Param body body LoginRequest true "credentials"
// @Success 200 {object} rest.R{data=rest.tokenPairView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/auth/login [POST]
func (s *Server) Login() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Return(c, nil, errs.Errf(errs.ErrBadRequest, "invalid JSON body: %s", err.Error()))

			return
		}

		pair, err := s.login.Execute(c.Request.Context(), req.Username, req.Password)
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toTokenPairView(pair), nil)
	}
}

// Logout godoc swagger
// @Summary signs the user out
// @Description - the access token used here and the refresh token stop working at once (401 afterwards);
// @Description   sign in again with POST /v1/auth/login
// @Tags auth
// @Security BearerAuth
// @Produce json
// @Success 200 {object} rest.R
// @Failure 401 {object} rest.UnauthorizedResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/auth/logout [POST]
func (s *Server) Logout() gin.HandlerFunc {
	return func(c *gin.Context) {
		Return(c, nil, s.logout.Execute(c.Request.Context(), currentUserID(c)))
	}
}

// ForgotPasswordRequest is the body of POST /v1/auth/forgot-password.
type ForgotPasswordRequest struct {
	PhoneNumber string `json:"phone_number" example:"+998901234567"`
	Password    string `json:"password" example:"n3w-s3cret-pass"`
}

// ForgotPassword godoc swagger
// @Summary sets a new password for a signed-out user, confirmed by an SMS code
// @Description - first POST /v1/otp/send and /v1/otp/verify with purpose reset_password for the
// @Description   user's phone number; without that verification: 403. A successful reset uses it up
// @Description - phone_number: +998 and 9 digits; password: 8 to 72 bytes, stored as a bcrypt hash (422)
// @Description - no user has this phone number: 404
// @Description - returns a new token pair (the user is signed in); every token issued before stops working
// @Tags auth
// @Accept json
// @Produce json
// @Param body body ForgotPasswordRequest true "phone number and new password"
// @Success 200 {object} rest.R{data=rest.tokenPairView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 403 {object} rest.ForbiddenResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/auth/forgot-password [POST]
func (s *Server) ForgotPassword() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ForgotPasswordRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Return(c, nil, errs.Errf(errs.ErrBadRequest, "invalid JSON body: %s", err.Error()))

			return
		}

		pair, err := s.forgotPassword.ExecuteByPhone(c.Request.Context(), req.PhoneNumber, req.Password)
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, toTokenPairView(pair), nil)
	}
}
