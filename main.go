package main

import (
	"os"

	"api.sjaplus.top/routes"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()

	config := cors.DefaultConfig()
	config.AllowOrigins = []string{"https://sjaplus.top"}
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE"}
	config.AllowHeaders = []string{"Origin", "Content-Type", "Accept"}
	r.Use(cors.New(config))

	// Setup routes
	routes.SetupRoutes(r)

	port := os.Getenv("BACKEND_PORT")
	if port == "" {
		port = "8080"
	}

	r.Run(":" + port)
}
