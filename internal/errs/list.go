package errs

var (
	ErrValidation   = New("validation error")
	ErrBadRequest   = New("bad request")
	ErrUnauthorized = New("unauthorized")
	ErrForbidden    = New("forbidden")
	ErrNotFound     = New("not found")
	ErrConflict     = New("conflict")
	ErrInternal     = New("internal error")
	ErrConnection   = New("connection error")
	ErrRateLimited  = New("too many requests")

	ErrActivityNotFound = Errf(ErrNotFound, "activity not found")
	ErrNoRecommendation = Errf(ErrNotFound, "no published activity matches this child's age and the filters")

	ErrArticleNotFound  = Errf(ErrNotFound, "article not found")
	ErrPostNotFound     = Errf(ErrNotFound, "post not found")
	ErrNoPost           = Errf(ErrNotFound, "no published video yet")
	ErrPostNotOpened    = Errf(ErrForbidden, "this video was not your daily video, the free plan opens one video a day")
	ErrVideoNotFound    = Errf(ErrValidation, "video_id is not an uploaded video, upload it to /v1/admin/media first")
	ErrNoArticle        = Errf(ErrNotFound, "no published article yet")
	ErrArticleNotOpened = Errf(ErrForbidden, "this article was not your daily article, the free plan opens one article a day")

	ErrUserNotFound     = Errf(ErrNotFound, "user not found")
	ErrUsernameTaken    = Errf(ErrConflict, "username is already taken")
	ErrPhoneNumberTaken = Errf(ErrConflict, "phone number is already taken")
	ErrPhotoNotFound    = Errf(ErrValidation, "photo_id is not an uploaded media id, upload the photo to /v1/media first")

	ErrChildNotFound  = Errf(ErrNotFound, "child not found")
	ErrNoPlanDayToday = Errf(ErrNotFound, "no plan day is open today: today's plan day is done, or the plan is done")
	ErrPlanNotDone    = Errf(ErrConflict, "the current plan is not done yet, the next plan starts after its last day")

	ErrTopicNotFound  = Errf(ErrNotFound, "topic not found")
	ErrTopicNameTaken = Errf(ErrConflict, "a topic with this name already exists")
	ErrUnknownTopic   = Errf(ErrValidation, "topic_id is not an active topic, list them with GET /v1/topics")
	ErrTipNotFound    = Errf(ErrNotFound, "tip not found")
	ErrNoTip          = Errf(ErrNotFound, "no published tip yet")

	ErrNeedsPro = Errf(ErrForbidden, "this needs Birga Pro")

	ErrGoalNotFound  = Errf(ErrNotFound, "goal not found")
	ErrGoalNameTaken = Errf(ErrConflict, "a goal with this name already exists")
	ErrUnknownGoals  = Errf(ErrValidation, "goals contains ids that are not active goals, list them with GET /v1/goals")

	ErrPhoneNumberRegistered    = Errf(ErrConflict, "phone number is already registered")
	ErrPhoneNumberNotRegistered = Errf(ErrNotFound, "phone number does not belong to a user")
	ErrOTPNotFound              = Errf(ErrNotFound, "code expired or was not requested, request a new one")
	ErrOTPTooManyAttempts       = Errf(ErrRateLimited, "too many wrong codes, request a new one")

	ErrInvalidRefreshToken = Errf(ErrUnauthorized, "refresh token is invalid or expired, sign in again")
	ErrRoleNotAllowed      = Errf(ErrForbidden, "your role cannot use this endpoint")
	ErrInvalidCredentials  = Errf(ErrUnauthorized, "username or password is wrong")

	ErrNewPhoneNotVerified = Errf(ErrForbidden, "new phone number is not verified, verify an update_user code for it first")
	ErrResetNotVerified    = Errf(ErrForbidden, "phone number is not verified, verify a reset_password code with /v1/otp/verify first")

	ErrMediaUnsupportedType        = Errf(ErrValidation, "unsupported file type, upload a JPEG, PNG or WebP image")
	ErrMediaUnsupportedTypeOrVideo = Errf(ErrValidation, "unsupported file type, upload a JPEG, PNG or WebP image or an MP4 or WebM video")
	ErrMediaEmpty                  = Errf(ErrValidation, "file is empty")
	ErrMediaNotFound               = Errf(ErrNotFound, "media not found")
)

var (
	ErrTooManyExtras  = Errf(ErrValidation, "a child's day holds at most 10 added activities")
	ErrNoExtra        = Errf(ErrNotFound, "no other published activity serves this goal at the child's age")
	ErrActivityInPlan = Errf(ErrConflict, "this activity is a day of the child's 7-day plan, do it on its plan day")

	ErrNotInToday        = Errf(ErrNotFound, "this activity is not in the child's list for today")
	ErrTodayItemDone     = Errf(ErrConflict, "this activity is already done today, it cannot be changed")
	ErrNoDifficultyMatch = Errf(ErrNotFound, "no other published activity of this difficulty serves the goal at the child's age")
)

// ErrUnknownSource rejects a content source that is not test, admin, harvard or ai.
var ErrUnknownSource = Errf(ErrValidation, "source must be test, admin, harvard or ai")

var (
	ErrMilestoneNotFound = Errf(ErrNotFound, "milestone not found")
	ErrUnknownActivities = Errf(ErrValidation, "activity_ids contains ids that are not activities")
)
