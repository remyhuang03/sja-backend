package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"
)

// Validates the id parameter from query string
//
// id should be a integer
func validateProjectID(c *gin.Context) (string, bool) {
	id := c.Query("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id parameter is required"})
		return "", false
	}

	// Validate that id is an integer
	if _, err := strconv.Atoi(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id must be a valid number"})
		return "", false
	}

	return id, true
}

// Handles requests for project avatar images
//
// API: /project/avatar?id=<id>
func ProjectAvatarHandler(c *gin.Context) {
	id, ok := validateProjectID(c)
	if !ok {
		return
	}

	// Construct the file path
	imagePath := filepath.Join("var", "project-display", "avatar", id+".png")

	// Check if file exists
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "avatar not found"})
		return
	}

	// Serve the image file
	c.File(imagePath)
}

// ProjectPosterHandler handles requests for project poster images
func ProjectPosterHandler(c *gin.Context) {
	id, ok := validateProjectID(c)
	if !ok {
		return
	}

	// Construct the file path
	imagePath := filepath.Join("var", "project-display", "poster", id+".png")

	// Check if file exists
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "poster not found"})
		return
	}

	// Serve the image file
	c.File(imagePath)
}
