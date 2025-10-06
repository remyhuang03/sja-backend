package routes

import (
	"api.sjaplus.top/handlers"
	"github.com/gin-gonic/gin"
)

// SetupRoutes configures all the routes for the application
func SetupRoutes(r *gin.Engine) {
	// Test route
	r.GET("/test", handlers.TestHandler)

	// Project application route
	r.POST("/project/apply", handlers.ProjectApplyHandler)

	// Project display routes
	r.GET("/project/avatar", handlers.ProjectAvatarHandler)
	r.GET("/project/poster", handlers.ProjectPosterHandler)
}
