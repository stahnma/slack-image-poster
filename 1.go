package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/nlopes/slack"
)

var (
	watcher *fsnotify.Watcher
	mu      sync.Mutex
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

	// Create a watcher
	watcher, _ = fsnotify.NewWatcher()
	defer watcher.Close()

	// Watch for events in the specified directory
	go watchDirectory(directoryPath)

	// Process images in the specified directory
	processImages(api, directoryPath, slackChannel)

	// Keep the program running
	select {}
}

func watchDirectory(directoryPath string) {
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			if event.Op&fsnotify.Create == fsnotify.Create {
				// If a new file is created, process it
				go handleNewFile(event.Name)
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			fmt.Println("Error watching directory:", err)
		}
	}
}

func handleNewFile(filePath string) {
	// Ensure the new file is an image
	if isImage(filepath.Base(filePath)) {
		mu.Lock()
		defer mu.Unlock()

		// Create a Slack client
		api := slack.New(os.Getenv("SLACK_TOKEN"))

		// Upload the new image to Slack
		err := uploadImageToSlack(api, filePath, os.Getenv("SLACK_CHANNEL"))
		if err != nil {
			fmt.Printf("File %s not uploaded. Error: %v\n", filepath.Base(filePath), err)
			return
		}

		// Move the processed image to the "processed" folder
		processedFolder := filepath.Join(filepath.Dir(filePath), "processed")
		err = os.MkdirAll(processedFolder, 0755)
		if err != nil {
			fmt.Printf("Error creating processed folder: %v\n", err)
			return
		}

		destPath := filepath.Join(processedFolder, filepath.Base(filePath))
		err = os.Rename(filePath, destPath)
		if err != nil {
			fmt.Printf("Error moving file %s to processed folder: %v\n", filepath.Base(filePath), err)
		} else {
			fmt.Printf("File %s uploaded to Slack and moved to processed folder.\n", filepath.Base(filePath))
		}
	}
}

func processImages(api *slack.Client, directoryPath, slackChannel string) {
	// List files in the specified directory
	files, err := ioutil.ReadDir(directoryPath)
	if err != nil {
		fmt.Println("Error reading directory:", err)
		os.Exit(1)
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
			processedFolder := filepath.Join(directoryPath, "processed")
			err = os.MkdirAll(processedFolder, 0755)
			if err != nil {
				fmt.Printf("Error creating processed folder: %v\n", err)
				continue
			}

			destPath := filepath.Join(processedFolder, file.Name())
			err = os.Rename(filePath, destPath)
			if err != nil {
				fmt.Printf("Error moving file %s to processed folder: %v\n", file.Name(), err)
			} else {
				fmt.Printf("File %s uploaded to Slack and moved to processed folder.\n", file.Name())
			}
		}
	}

	// Sleep for a while before processing again
	time.Sleep(5 * time.Second)
	processImages(api, directoryPath, slackChannel)
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

