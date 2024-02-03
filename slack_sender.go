package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/fsnotify/fsnotify"
	"github.com/slack-go/slack"
	"github.com/spf13/viper"
)

var (
	watcher *fsnotify.Watcher
	mu      sync.Mutex
)

func watchDirectory(directoryPath string, done chan struct{}) {
	log.Debugln("(watchDirectory)", directoryPath)
	if err := watcher.Add(directoryPath); err != nil {
		log.Errorln("Error watching directory:", err)
		return
	}
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				log.Debugln("(watchingDirectory) watcher.Events channel closed. Exiting watchDirectory.")
				return
			}

			if event.Op&fsnotify.Create == fsnotify.Create {
				go handleNewFile(event.Name)
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				log.Debugln("(watchDirectory) watcher.Errors channel closed. Exiting watchDirectory.")
				return
			}
			log.Errorln("Error watching directory:", err)

		case <-done:
			log.Debugln("(watchDirectory) Received signal to exit. Exiting watchDirectory.")
			return
		}

	}
}

func handleNewFile(filePath string) {
	// if it's not an image, use the json file to see comment

	log.Debugln("(handleNewFile) filePath:", filePath)
	mu.Lock()
	defer mu.Unlock()

	if isImage(filePath) {
		return
	}

	if isJson(filePath) {
		log.Debugln("(handleNewfile) Found a json file", filePath)
		var j ImageInfo
		var err error
		content, err := os.ReadFile(filePath)
		if err != nil {
			log.Debugln("(handleNewFile) Can't read the file", filePath)
		}
		err = json.Unmarshal(content, &j)
		if err != nil {
			// move the file to the invalid folder
			log.Warnln("Unable to process " + filePath + " moving to invalid folder")
			log.Debugln("(handleNewFile) Json unmarhsalling error is ", err)
			return
		}
		handleImageFile(j)
		moveToDir(filePath, viper.GetString("processed_dir"))
	}
}

func handleImageFile(j ImageInfo) {
	api := slack.New(viper.GetString("slack_token"))
	filePath := j.ImagePath
	// Ensure the new file is an image
	if isImage(filepath.Base(filePath)) {
		// Upload the new image to Slack
		err := uploadImageToSlack(api, j, viper.GetString("slack_channel"))
		if err != nil {
			log.Errorln("File %s not uploaded. Error: %v\n", filepath.Base(filePath), err)
			return
		}
		moveToDir(filePath, viper.GetString("processed_dir"))
	}
}

func uploadImageToSlack(api *slack.Client, j ImageInfo, slackChannel string) error {
	filePath := j.ImagePath
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	var comment, author string
	if j.Caption == "" {
		comment = "New image uploaded to Slack!"
	} else {
		comment = j.Caption
	}
	if j.Author == "" {
		author = "Anonymous"
	} else {
		author = "Routine to be called"
	}

	params := slack.FileUploadParameters{
		File:           filePath,
		Filename:       filepath.Base(filePath),
		Filetype:       "auto",
		Title:          author,
		Channels:       []string{slackChannel},
		InitialComment: comment,
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

func hasJsonExtension(fileName string) bool {
	extensions := []string{".json"}
	lowerCaseFileName := strings.ToLower(fileName)
	for _, ext := range extensions {
		if strings.HasSuffix(lowerCaseFileName, ext) {
			return true
		}
	}
	return false
}

// FIXME - handle sceanrio where a spare image is just in the upload dir
func handleSpareImage(filePath string) {
	log.Debugln("(handleSpareImage) filePath: ", filePath)
	mu.Lock()
	defer mu.Unlock()
	moveToDir(filePath, viper.GetString("discard_dir"))
}

func isJson(filename string) bool {
	log.Debugln("(isJson) filename:", filename)
	if hasJsonExtension(filename) {
		log.Debugln(filename + " Has Json extension")
		content, err := os.ReadFile(filename)
		if err != nil {
			log.Debugln("(isJson) Unable to read file", filename)
			return false
		}
		// See if it's valid JSON
		var jsonData interface{}
		err = json.Unmarshal(content, &jsonData)
		if err != nil {
			log.Debugln("(isJson) Unable to parse json. Error is", err)
			moveToDir(filename, viper.GetString("discard_dir"))
			log.Infoln("Moved " + filename + " to discard directory. Invalid JSON file or schema.")
			return false
		}
		return true
	}
	log.Debugln("(isJson) " + filename + "Not a json file")
	return false
}
