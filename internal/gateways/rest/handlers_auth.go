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
}

type tokenPairView struct {
	AccessToken  string `json:"access_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	RefreshToken string `json:"refresh_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	ExpiresIn    int    `json:"expires_in" example:"900"` // seconds until the access token expires
}

// SignUp godoc swagger
// @Summary creates a user and returns their tokens
// @Description - the phone number must be verified first: POST /v1/otp/send and /v1/otp/verify with
// @Description   purpose sign_up; the verification lasts OTP_VERIFIED_TTL (default 10 minutes) and is
// @Description   used up by a successful sign-up. Not verified: 403
// @Description - name: 1 to 100 characters; username: 3 to 32 of a-z, 0-9, '_' or '.' (lowercased);
// @Description   password: 8 to 72 bytes, stored as a bcrypt hash; phone_number: +998 and 9 digits (422)
// @Description - username or phone number already used: 409
// @Description - access_token (JWT_ACCESS_TTL, default 15 minutes) and refresh_token (JWT_REFRESH_TTL,
// @Description   default 30 days) are HS256 JWTs; send the access token as Authorization: Bearer <token>
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
		})
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, tokenPairView{
			AccessToken:  pair.AccessToken,
			RefreshToken: pair.RefreshToken,
			ExpiresIn:    int(pair.AccessExpiresIn.Seconds()),
		}, nil)
	}
}

// RefreshTokenRequest is the body of POST /v1/auth/refresh.
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
}

type accessTokenView struct {
	AccessToken string `json:"access_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	ExpiresIn   int    `json:"expires_in" example:"900"` // seconds until the access token expires
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
