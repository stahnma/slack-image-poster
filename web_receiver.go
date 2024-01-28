package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
)

// ImageInfo represents the data to be stored in the JSON file
type ImageInfo struct {
	ImagePath string `json:"image_path"`
	Caption   string `json:"caption"`
}

func uploadHandler(c *gin.Context) {
	// Check for the presence of the X-API-Key header
	apiKey := c.GetHeader("X-API-Key")
	expectedKey := os.Getenv("API_KEY") // Read API key from environment variable

	if apiKey != expectedKey {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		c.Error(fmt.Errorf("Unauthorized request from IP: %s", c.ClientIP()))
		return
	}

	// Capture the start time of the request
	startTime := time.Now()

	// Parse the form data, including files
	err := c.Request.ParseMultipartForm(10 << 20) // 10 MB limit
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unable to parse form"})
		c.Error(fmt.Errorf("Error parsing form data: %v (Status: %d)", err, http.StatusBadRequest))
		return
	}

	// Get the image file and other form data
	image, imageHeader, err := c.Request.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unable to get image"})
		c.Error(fmt.Errorf("Error getting image file: %v (Status: %d)", err, http.StatusBadRequest))
		return
	}
	defer image.Close()

	caption := c.Request.FormValue("caption")

	// Create a unique filename for the uploaded image
	imageName := fmt.Sprintf("%d_%s", getCurrentTimestamp(), imageHeader.Filename)
	uploadDir := os.Getenv("UPLOAD_DIR") // Read upload directory from environment variable
	imagePath := filepath.Join(uploadDir, imageName)
	file, err := os.Create(imagePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to create file"})
		c.Error(fmt.Errorf("Error creating file: %v (Status: %d)", err, http.StatusInternalServerError))
		return
	}
	defer file.Close()

	// Save the image to disk
	_, err = io.Copy(file, image)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to save image"})
		c.Error(fmt.Errorf("Error saving image file: %v (Status: %d)", err, http.StatusInternalServerError))
		return
	}

	// Create and save the JSON file
	jsonPath := filepath.Join(uploadDir, fmt.Sprintf("%s.json", imageName))
	imageInfo := ImageInfo{ImagePath: imagePath, Caption: caption}
	jsonData, err := json.MarshalIndent(imageInfo, "", "    ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to create JSON"})
		c.Error(fmt.Errorf("Error creating JSON: %v (Status: %d)", err, http.StatusInternalServerError))
		return
	}
	err = os.WriteFile(jsonPath, jsonData, 0644)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to save JSON"})
		c.Error(fmt.Errorf("Error saving JSON file: %v (Status: %d)", err, http.StatusInternalServerError))
		return
	}

	// Respond with success message
	c.JSON(http.StatusOK, gin.H{"message": "Upload successful", "filename": imageName})

	// Log the upload in the same format as [GIN] logging lines
	fmt.Fprintf(gin.DefaultWriter, "[GIN] %s | %3d | %13v | %15s | %-7s %s | Filename: %s\n",
		startTime.Format("2006/01/02 - 15:04:05"),
		http.StatusOK,
		time.Since(startTime),
		c.ClientIP(),
		c.Request.Method,
		c.Request.URL.Path,
		imageName,
	)
}

func getCurrentTimestamp() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

func receiver(done chan struct{}) {
	// Read environment variables
	uploadDir := os.Getenv("UPLOAD_DIR")
	apiKey := os.Getenv("API_KEY")
	port := os.Getenv("UPLOAD_PORT")

	// Validate environment variables
	if uploadDir == "" {
		log.Fatal("UPLOAD_DIR environment variable not set.")
	}

	if apiKey == "" {
		log.Fatal("API_KEY environment variable not set.")
	}

	if port == "" {
		port = "8080" // Default port
	}

	// Set up Gin with custom logging middleware
	router := gin.New()

	// Set up the /upload route
	router.POST("/upload", uploadHandler)

	// Start the server
	router.Run(":" + port)
}
