package rest

import (
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "gitlab.com/loyihalar/birga/backend/api/docs" // required by swagger to load generated docs files
)

func (s *Server) endpoints() {
	s.router.GET("/ping", s.Ping())
	s.router.GET("/health", s.Health())
	s.router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	v1 := s.router.Group("/v1")
	{
		v1.GET("/activities", s.ListActivities())
		v1.GET("/activities/:id", s.GetActivity())
	}

	admin := v1.Group("/admin", s.adminAuth())
	{
		admin.POST("/activities", s.CreateActivity())
		admin.GET("/activities", s.AdminListActivities())
		admin.GET("/activities/:id", s.AdminGetActivity())

		admin.POST("/users", s.CreateUser())
		admin.GET("/users", s.ListUsers())
		admin.GET("/users/:id", s.GetUser())
		admin.PATCH("/users/:id", s.UpdateUser())
		admin.DELETE("/users/:id", s.DeleteUser())
	}
}
