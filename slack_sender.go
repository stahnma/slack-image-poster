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
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var (
	watcher *fsnotify.Watcher
	mu      sync.Mutex
)

func init() {
	// Set up Viper to use command line flags
	pflag.String("config", "", "config file (default is $HOME/.your_app.yaml)")
	pflag.Bool("ready-systemd", false, "flag to indicate systemd readiness")

	// Bind the command line flags to Viper
	viper.BindPFlags(pflag.CommandLine)
}

func watchDirectory(directoryPath string, done chan struct{}) {
	if err := watcher.Add(directoryPath); err != nil {
		fmt.Println("Error watching directory:", err)
		return
	}
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				fmt.Println("watcher.Events channel closed. Exiting watchDirectory.")
				return
			}

			// Useful for debugging
			// fmt.Println("Event received:", event)

			if event.Op&fsnotify.Create == fsnotify.Create {
				go handleNewFile(event.Name)
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				fmt.Println("watcher.Errors channel closed. Exiting watchDirectory.")
				return
			}
			fmt.Println("Error watching directory:", err)
		case <-done:
			fmt.Println("Received signal to exit. Exiting watchDirectory.")
			return

		}
	}
}

func handleNewFile(filePath string) {
	// Ensure the new file is an image
	if isImage(filepath.Base(filePath)) {
		mu.Lock()
		defer mu.Unlock()

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
			//	fmt.Printf("File %s uploaded to Slack and moved to processed folder.\n", filepath.Base(filePath))
			t := time.Now()
			fmt.Printf("[SND] %s %s sent to Slack and moved to \"processed\" directory.\n", t.Format("2006/01/02 - 15:04:05"), filepath.Base(filePath))
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
		// Check if the file is an image
		if isImage(file.Name()) {
			filePath := filepath.Join(directoryPath, file.Name())
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
				t := time.Now()
				fmt.Printf("[Upload] %s %s uploaded to Slack and moved to processed folder.\n", t.Format("2006/01/02 - 15:04:05"), file.Name())
			}
		}
	}

	// Sleep for a while before processing again
	time.Sleep(5 * time.Second)
	processImages(api, directoryPath, slackChannel)
}

func uploadImageToSlack(api *slack.Client, filePath, slackChannel string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	params := slack.FileUploadParameters{
		File:           filePath,
		Filename:       filepath.Base(filePath),
		Filetype:       "auto",
		Title:          "Uploaded Image",
		Channels:       []string{slackChannel},
		InitialComment: "New image uploaded to Slack!",
	}

	_, err = api.UploadFile(params)
	if err != nil {
		return err
	}

	return nil
}

func isImage(fileName string) bool {
	extensions := []string{".jpg", ".jpeg", ".png", ".gif"}
	lowerCaseFileName := strings.ToLower(fileName)
	for _, ext := range extensions {
		if strings.HasSuffix(lowerCaseFileName, ext) {
			return true
		}
	}
	return false
}
