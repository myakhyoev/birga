package rest

import (
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "gitlab.com/loyihalar/birga/backend/api/docs" // required by swagger to load generated docs files
	"gitlab.com/loyihalar/birga/backend/internal/domain"
)

func (s *Server) endpoints() {
	s.router.GET("/ping", s.Ping())
	s.router.GET("/health", s.Health())
	s.router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	v1 := s.router.Group("/v1")
	{
		v1.GET("/activities", s.ListActivities())
		v1.GET("/activities/:id", s.GetActivity())

		// Open without a token: the app shows the goals on the sign-up screen.
		v1.GET("/goals", s.ListGoals())
		v1.GET("/goals/:id", s.GetGoal())
		v1.GET("/topics", s.ListTopics())

		v1.POST("/otp/send", s.SendOTP())
		v1.POST("/otp/verify", s.VerifyOTP())

		v1.POST("/auth/signup", s.SignUp())
		v1.POST("/auth/refresh", s.RefreshToken())
		v1.POST("/auth/login", s.Login())
		v1.POST("/auth/forgot-password", s.ForgotPassword())

		v1.POST("/media", s.UploadMedia())
		v1.GET("/media/:id", s.GetMedia())
	}

	v1.POST("/auth/logout", s.userAuth(), s.Logout())
	v1.GET("/pro", s.userAuth(), s.GetProOffer())
	v1.POST("/pro/purchase", s.userAuth(), s.PurchasePro())
	v1.GET("/tips", s.userAuth(), s.ListTips())
	v1.GET("/activities/:id/similar", s.userAuth(), s.GetSimilarActivities())
	v1.GET("/milestones/:id", s.userAuth(), s.GetMilestone())

	// Every signed-in role may manage its own children; ownership is checked per child in the use cases.
	v1.POST("/children",
		s.userAuth(domain.UserRoleUnverifiedUser, domain.UserRoleUser, domain.UserRolePaidUser, domain.UserRoleAdmin),
		s.CreateChild())

	// The signed-in user's own account. Every role may use it.
	me := v1.Group("/me", s.userAuth())
	{
		me.GET("", s.GetProfile())
		me.PATCH("", s.UpdateProfile())
		me.DELETE("", s.DeleteAccount())
		me.PUT("/password", s.ResetPassword())
		me.GET("/children", s.ListMyChildren())
		me.GET("/articles", s.ListMyArticles())
		me.GET("/articles/daily", s.GetDailyArticle())
		me.GET("/posts", s.ListMyPosts())
		me.GET("/posts/daily", s.GetDailyPost())
		me.GET("/tips/daily", s.GetDailyTip())
	}

	articles := v1.Group("/articles", s.userAuth())
	{
		articles.GET("", s.ListArticles())
		articles.GET("/:id", s.GetArticle())
		articles.POST("/:id/read", s.MarkArticleRead())
	}

	posts := v1.Group("/posts", s.userAuth())
	{
		posts.GET("", s.ListPosts())
		posts.GET("/:id", s.GetPost())
	}

	child := v1.Group("/children/:id", s.userAuth())
	{
		child.GET("", s.GetChild())
		child.PATCH("", s.UpdateChild())
		child.GET("/plan", s.GetPlan())
		child.POST("/plan", s.RestartPlan())
		child.GET("/recommendation", s.RecommendActivity())
		child.GET("/today", s.GetToday())
		child.POST("/today/extras", s.AddTodayExtra())
		child.POST("/today/difficulty", s.ChangeTodayDifficulty())
		child.GET("/milestones", s.GetChildMilestones())
		child.PUT("/milestones/:milestone_id", s.MarkMilestone())
		child.DELETE("/milestones/:milestone_id", s.UnmarkMilestone())
		child.GET("/report", s.GetMonthlyReport())
		child.POST("/completions", s.CompleteActivity())
		child.GET("/completions", s.ListCompletions())
		child.GET("/streak", s.GetStreak())
		child.GET("/streak/days", s.GetStreakDays())
	}

	admin := v1.Group("/admin", s.adminAuth())
	{
		admin.POST("/activities", s.CreateActivity())
		admin.GET("/activities", s.AdminListActivities())
		admin.GET("/activities/:id", s.AdminGetActivity())
		admin.PATCH("/activities/:id", s.UpdateActivity())
		admin.DELETE("/activities/:id", s.DeleteActivity())

		admin.POST("/articles", s.CreateArticle())
		admin.GET("/articles", s.AdminListArticles())
		admin.GET("/articles/:id", s.AdminGetArticle())
		admin.PATCH("/articles/:id", s.UpdateArticle())
		admin.DELETE("/articles/:id", s.DeleteArticle())

		admin.POST("/posts", s.CreatePost())
		admin.GET("/posts", s.AdminListPosts())
		admin.GET("/posts/:id", s.AdminGetPost())
		admin.PATCH("/posts/:id", s.UpdatePost())
		admin.DELETE("/posts/:id", s.DeletePost())

		admin.POST("/tips", s.CreateTip())
		admin.GET("/tips", s.AdminListTips())
		admin.GET("/tips/:id", s.AdminGetTip())
		admin.PATCH("/tips/:id", s.UpdateTip())
		admin.DELETE("/tips/:id", s.DeleteTip())

		admin.POST("/milestones", s.CreateMilestone())
		admin.GET("/milestones", s.AdminListMilestones())
		admin.GET("/milestones/:id", s.AdminGetMilestone())
		admin.PATCH("/milestones/:id", s.UpdateMilestone())
		admin.DELETE("/milestones/:id", s.DeleteMilestone())

		admin.POST("/topics", s.CreateTopic())
		admin.PATCH("/topics/:id", s.UpdateTopic())
		admin.DELETE("/topics/:id", s.DeleteTopic())

		admin.POST("/media", s.AdminUploadMedia())
		admin.POST("/media/:id/recrop", s.AdminRecropMedia())

		admin.POST("/goals", s.CreateGoal())
		admin.PATCH("/goals/:id", s.UpdateGoal())
		admin.DELETE("/goals/:id", s.DeleteGoal())

		admin.POST("/users", s.CreateUser())
		admin.GET("/users", s.ListUsers())
		admin.GET("/users/:id", s.GetUser())
		admin.PATCH("/users/:id", s.UpdateUser())
		admin.DELETE("/users/:id", s.DeleteUser())
		admin.PUT("/users/:id/pro", s.SetUserPro())
	}
}
