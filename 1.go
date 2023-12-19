package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nlopes/slack"
)

func main() {
	// Get Slack API token, directory path, Slack channel from environment variables
	slackToken := os.Getenv("SLACK_TOKEN")
	directoryPath := os.Getenv("IMAGE_DIRECTORY")
	slackChannel := os.Getenv("SLACK_CHANNEL")

	if slackToken == "" || directoryPath == "" || slackChannel == "" {
		fmt.Println("Please set SLACK_TOKEN, IMAGE_DIRECTORY, and SLACK_CHANNEL environment variables.")
		os.Exit(1)
	}

	// Create a Slack client
	api := slack.New(slackToken)

	// Process image files in the specified directory
	err := processImages(api, directoryPath, slackChannel)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}

func processImages(api *slack.Client, directoryPath, slackChannel string) error {
	// Create a "processed" folder if it doesn't exist
	processedFolder := filepath.Join(directoryPath, "processed")
	if err := os.MkdirAll(processedFolder, 0755); err != nil {
		return err
	}

	// List files in the specified directory
	files, err := ioutil.ReadDir(directoryPath)
	if err != nil {
		return err
	}

	// Sort files by creation time (oldest first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime().Before(files[j].ModTime())
	})

	// Iterate through files in the directory
	for _, file := range files {
		// Check if the file is an image (you can customize the check based on your needs)
		if isImage(file.Name()) {
			// Prepare file path
			filePath := filepath.Join(directoryPath, file.Name())

			// Upload the image to Slack
			err := uploadImageToSlack(api, filePath, slackChannel)
			if err != nil {
				fmt.Printf("File %s not uploaded. Error: %v\n", file.Name(), err)
				continue
			}

			// Move the processed image to the "processed" folder
			destPath := filepath.Join(processedFolder, file.Name())
			err = os.Rename(filePath, destPath)
			if err != nil {
				fmt.Printf("Error moving file %s to processed folder: %v\n", file.Name(), err)
			} else {
				fmt.Printf("File %s uploaded to Slack and moved to processed folder.\n", file.Name())
			}
		}
	}

	return nil
}

func uploadImageToSlack(api *slack.Client, filePath, slackChannel string) error {
	// Open the image file
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Prepare the file upload parameters
	params := slack.FileUploadParameters{
		File:           filePath, // Use file path here
		Filename:       filepath.Base(filePath),
		Filetype:       "auto",
		Title:          "Uploaded Image",
		Channels:       []string{slackChannel},
		InitialComment: "New image uploaded to Slack!",
	}

	// Upload the image to Slack
	_, err = api.UploadFile(params)
	if err != nil {
		return err
	}

	return nil
}

func isImage(fileName string) bool {
	// You can customize this function based on the file extensions you want to consider as images
	extensions := []string{".jpg", ".jpeg", ".png", ".gif"}
	lowerCaseFileName := strings.ToLower(fileName)
	for _, ext := range extensions {
		if strings.HasSuffix(lowerCaseFileName, ext) {
			return true
		}
	}
	return false
}

