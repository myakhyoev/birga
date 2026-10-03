package rest

import (
	"github.com/gin-gonic/gin"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

// SendOTPRequest is the body of POST /v1/otp/send.
type SendOTPRequest struct {
	PhoneNumber string `json:"phone_number" example:"+998901234567"`
	Purpose     string `json:"purpose" enums:"sign_up,update_user" example:"sign_up"`
	IPAddress   string `json:"ip_address" example:"203.0.113.7"`
}

type sendOTPView struct {
	ExpiresIn int `json:"expires_in" example:"180"` // seconds until the code expires
	ResendIn  int `json:"resend_in" example:"60"`   // seconds until a new code may be requested
}

// SendOTP godoc swagger
// @Summary sends a one-time code by SMS
// @Description - phone_number: an Uzbek mobile number in E.164, +998 and 9 digits (422 otherwise)
// @Description - purpose: sign_up (the number must not belong to a user, 409 otherwise) or update_user
// @Description - ip_address: the end user's IPv4 or IPv6 address, used for rate limits
// @Description - the code has 6 digits and is sent through Play Mobile; only its hash is stored
// @Description - the code is kept in Redis for expires_in seconds (OTP_TTL, default 2 minutes); a new
// @Description   code replaces the previous one for the same phone and purpose
// @Description - rate limiter (429): one code per phone and purpose per resend_in seconds, and per hour
// @Description   at most OTP_MAX_PER_PHONE_HOUR codes per phone and OTP_MAX_PER_IP_HOUR per IP
// @Tags otp
// @Accept json
// @Produce json
// @Param body body SendOTPRequest true "where and why to send the code"
// @Success 200 {object} rest.R{data=rest.sendOTPView}
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 409 {object} rest.ConflictResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 429 {object} rest.TooManyRequestsResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/otp/send [POST]
func (s *Server) SendOTP() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req SendOTPRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Return(c, nil, errs.Errf(errs.ErrBadRequest, "invalid JSON body: %s", err.Error()))

			return
		}

		res, err := s.otpSender.Execute(c.Request.Context(), domain.OTPSendRequest{
			PhoneNumber: req.PhoneNumber,
			Purpose:     domain.OTPPurpose(req.Purpose),
			IPAddress:   req.IPAddress,
		})
		if err != nil {
			Return(c, nil, err)

			return
		}

		Return(c, sendOTPView{ExpiresIn: int(res.ExpiresIn.Seconds()), ResendIn: int(res.ResendIn.Seconds())}, nil)
	}
}

// VerifyOTPRequest is the body of POST /v1/otp/verify.
type VerifyOTPRequest struct {
	PhoneNumber string `json:"phone_number" example:"+998901234567"`
	Purpose     string `json:"purpose" enums:"sign_up,update_user" example:"sign_up"`
	Code        string `json:"code" example:"480569"`
}

// VerifyOTP godoc swagger
// @Summary checks a one-time code
// @Description - phone_number and purpose must be the ones the code was sent for; code is 6 digits (422)
// @Description - a matching code is deleted, so it works only once; data is null
// @Description - wrong code: 422 with the attempts left; after OTP_MAX_VERIFY_ATTEMPTS wrong codes the
// @Description   code is deleted and the answer is 429
// @Description - no code (never sent, expired after 2 minutes, or already used): 404
// @Description - OTP_DEFAULT_CODE, when configured (never in production), is accepted for any phone
// @Description   and purpose without checking
// @Tags otp
// @Accept json
// @Produce json
// @Param body body VerifyOTPRequest true "code to check"
// @Success 200 {object} rest.R
// @Failure 400 {object} rest.BadRequestResponse
// @Failure 404 {object} rest.NotFoundResponse
// @Failure 422 {object} rest.UnprocessableContentResponse
// @Failure 429 {object} rest.TooManyRequestsResponse
// @Failure 500 {object} rest.InternalServerErrorResponse
// @Router /v1/otp/verify [POST]
func (s *Server) VerifyOTP() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req VerifyOTPRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Return(c, nil, errs.Errf(errs.ErrBadRequest, "invalid JSON body: %s", err.Error()))

			return
		}

		Return(c, nil, s.otpVerifier.Execute(c.Request.Context(), domain.OTPVerifyRequest{
			PhoneNumber: req.PhoneNumber,
			Purpose:     domain.OTPPurpose(req.Purpose),
			Code:        req.Code,
		}))
	}
}
