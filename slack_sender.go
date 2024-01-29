package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"

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

	// Set up logging
	log.SetOutput(os.Stdout)
	log.SetLevel(log.DebugLevel)
	//log.SetLevel(log.InfoLevel)

	// Set up Viper to use command line flags
	pflag.String("config", "", "config file (default is $HOME/.your_app.yaml)")
	pflag.Bool("ready-systemd", false, "flag to indicate systemd readiness")

	// Bind the command line flags to Viper
	viper.BindPFlags(pflag.CommandLine)

	//TODO move all configuration handling to here
}

func watchDirectory(directoryPath string, done chan struct{}) {
	setupDirectory(directoryPath)
	setupDirectory(directoryPath + "/../processed")
	setupDirectory(directoryPath + "/../discard")
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
	// if it's not an image, use the json file to see comment

	log.Debugln("Inside handleNewFile", filePath)
	mu.Lock()
	defer mu.Unlock()

	if isImage(filePath) {
		return
	}

	if isJson(filePath) {
		log.Debugln("Found a json file", filePath)
		var j ImageInfo
		var err error
		content, err := ioutil.ReadFile(filePath)
		if err != nil {
			log.Debugln("I can't read the file", filePath)
		}
		err = json.Unmarshal(content, &j)
		if err != nil {
			// move the file to the invalid folder
			log.Warnln("Unable to process " + filePath + " moving to invalid folder")
			log.Debugln("Json unmarhsalling error is ", err)
			return
		}
		handleImageFile(j)
		moveToDir(filePath, "processed")
	}
}

func handleImageFile(j ImageInfo) {
	api := slack.New(os.Getenv("SLACK_TOKEN"))
	filePath := j.ImagePath
	// Ensure the new file is an image
	if isImage(filepath.Base(filePath)) {
		// Upload the new image to Slack
		err := uploadImageToSlack(api, j, os.Getenv("SLACK_CHANNEL"))
		if err != nil {
			fmt.Printf("File %s not uploaded. Error: %v\n", filepath.Base(filePath), err)
			return
		}
		moveToDir(filePath, "processed")
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

func isJson(filename string) bool {
	log.Debugln("Inside isJson", filename)
	if hasJsonExtension(filename) {
		log.Debugln(filename + " Has Json extension")
		content, err := ioutil.ReadFile(filename)
		if err != nil {
			log.Debugln("I can't read the file", filename)
			return false
		}
		// See if it's valid JSON
		var jsonData interface{}
		err = json.Unmarshal(content, &jsonData)
		log.Debugln("Unable to parse json. Error is ", err)
		moveToDir(filename, "discard")
		log.Infoln("Moved " + filename + " to discard directory. Invalid JSON file or schema.")
		return err == nil
	}
	log.Debugln(filename + "Not a json file")
	return false
}
