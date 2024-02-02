package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// ImageInfo represents the data to be stored in the JSON file
type ImageInfo struct {
	ImagePath string `json:"image_path"`
	Caption   string `json:"caption"`
	Author    string `json:"author"`
}

func uploadHandler(c *gin.Context) {
	// FIXME need logging throughout
	log.Debugln("(uploadHandler) Inside uploadHandler")
	// Check for the presence of the X-API-Key header
	apiKey := c.GetHeader("X-API-Key")
	// FIXME move retreival for this key to viper
	// figure out what if the provided key is valid
	isApiKeyValied, err := validateApiKey(apiKey)
	if err != nil {
		log.Debug("(uploadHandler) validateApiKey threw error")
		if err.Error() != "Key has been revoked" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err})
			return
		}
	}
	if isApiKeyValied {
		log.Debugln("API key is valid")
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		c.Error(fmt.Errorf("Unauthorized request from IP: %s", c.ClientIP()))
		log.Warnln("Request has invalid API key")
		return
	}

	startTime := time.Now()

	err = c.Request.ParseMultipartForm(10 << 20) // 10 MB limit
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unable to parse form"})
		c.Error(fmt.Errorf("Error parsing form data: %v (Status: %d)", err, http.StatusBadRequest))
		return
	}

	image, imageHeader, err := c.Request.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unable to get image"})
		c.Error(fmt.Errorf("Error getting image file: %v (Status: %d)", err, http.StatusBadRequest))
		return
	}
	defer image.Close()

	caption := c.Request.FormValue("caption")
	imageName := fmt.Sprintf("%d_%s", getCurrentTimestamp(), imageHeader.Filename)
	uploadDir := viper.GetString("uploads_dir")
	setupDirectory(uploadDir)
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

	// TODO - remove the img extension from the json filename
	jsonPath := filepath.Join(uploadDir, fmt.Sprintf("%s.json", imageName))
	imageInfo := ImageInfo{ImagePath: imagePath, Caption: caption}
	jsonData, err := json.MarshalIndent(imageInfo, "", "    ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to create JSON"})
		c.Error(fmt.Errorf("Error creating JSON: %v (Status: %d)", err, http.StatusInternalServerError))
		return
	}
	err = os.WriteFile(jsonPath, jsonData, 0o644)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to save JSON"})
		c.Error(fmt.Errorf("Error saving JSON file: %v (Status: %d)", err, http.StatusInternalServerError))
		return
	}

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
	router := gin.New()
	router.POST("/upload", uploadHandler)
	router.POST("/api", apiEndpoint)
	router.DELETE("/api", apiEndpoint)
	router.Run(":" + viper.GetString("PORT"))
}
