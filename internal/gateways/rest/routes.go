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

		v1.POST("/otp/send", s.SendOTP())
		v1.POST("/otp/verify", s.VerifyOTP())

		v1.POST("/auth/signup", s.SignUp())
		v1.POST("/auth/refresh", s.RefreshToken())

		v1.POST("/media", s.UploadMedia())
	}

	// Every signed-in role may manage its own children; ownership is checked per child in the use cases.
	v1.POST("/children", s.userAuth(domain.UserRoleUser, domain.UserRolePaidUser, domain.UserRoleAdmin), s.CreateChild())

	// The signed-in user's own account. Every role may use it.
	me := v1.Group("/me", s.userAuth())
	{
		me.GET("", s.GetProfile())
		me.PATCH("", s.UpdateProfile())
		me.DELETE("", s.DeleteAccount())
		me.PUT("/password", s.ResetPassword())
		me.GET("/children", s.ListMyChildren())
	}

	child := v1.Group("/children/:id", s.userAuth())
	{
		child.GET("", s.GetChild())
		child.GET("/recommendation", s.RecommendActivity())
		child.POST("/completions", s.CompleteActivity())
		child.GET("/completions", s.ListCompletions())
		child.GET("/streak", s.GetStreak())
	}

	admin := v1.Group("/admin", s.adminAuth())
	{
		admin.POST("/activities", s.CreateActivity())
		admin.GET("/activities", s.AdminListActivities())
		admin.GET("/activities/:id", s.AdminGetActivity())
		admin.PATCH("/activities/:id", s.UpdateActivity())
		admin.DELETE("/activities/:id", s.DeleteActivity())

		admin.POST("/users", s.CreateUser())
		admin.GET("/users", s.ListUsers())
		admin.GET("/users/:id", s.GetUser())
		admin.PATCH("/users/:id", s.UpdateUser())
		admin.DELETE("/users/:id", s.DeleteUser())
	}
}
