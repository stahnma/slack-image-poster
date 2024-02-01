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

	// Set up Viper to use command line flags
	pflag.String("config", "", "config file (default is $HOME/.your_app.yaml)")
	pflag.Bool("ready-systemd", false, "flag to indicate systemd readiness")

	// Bind the command line flags to Viper
	viper.BindPFlags(pflag.CommandLine)

	// FIXME move other config here: slackToken, slackChannel, etc.
	viper.SetDefault("port", "8080")
	viper.SetDefault("data_dir", "data")
	viper.BindEnv("port", "PORT")
	viper.BindEnv("data_dir", "DATA_DIR")

	var bugout bool
	if value := os.Getenv("SLACK_TOKEN"); value == "" {
		fmt.Println("SLACK_TOKEN environment variable not set.")
		bugout = true
	}
	if value := os.Getenv("SLACK_CHANNEL"); value == "" {
		fmt.Println("SLACK_CHANNEL environment variable not set.")
		bugout = true
	}
	if bugout == true {
		os.Exit(1)
	}

	viper.MustBindEnv("slack_token", "SLACK_TOKEN")
	viper.MustBindEnv("slack_channel", "SLACK_CHANNEL")

	viper.SetDefault("discard_dir", viper.GetString("data_dir")+"/discard")
	viper.SetDefault("processed_dir", viper.GetString("data_dir")+"/processed")
	viper.SetDefault("uploads_dir", viper.GetString("data_dir")+"/uploads")
	viper.SetDefault("credentials_dir", viper.GetString("data_dir")+"/credentials")

	viper.BindEnv("data_dir", "DATA_DIR")
	viper.BindEnv("discard_dir", "DISCARD_DIR")
	viper.BindEnv("processed_dir", "PROCESSED_DIR")
	viper.BindEnv("uploads_dir", "UPLOADS_DIR")
	viper.BindEnv("credentials_dir", "CREDENTIALS_DIR")

	setupDirectory(viper.GetString("data_dir"))
	setupDirectory(viper.GetString("discard_dir"))
	setupDirectory(viper.GetString("processed_dir"))
	setupDirectory(viper.GetString("uploads_dir"))
	setupDirectory(viper.GetString("credentials_dir"))

}

func watchDirectory(directoryPath string, done chan struct{}) {
	log.Debugln("Inside watchDirectory", directoryPath)
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
			fmt.Printf("File %s not uploaded. Error: %v\n", filepath.Base(filePath), err)
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
	log.Debugln("Inside handleSpareImage", filePath)
	mu.Lock()
	defer mu.Unlock()
	moveToDir(filePath, viper.GetString("discard_dir"))
}

func isJson(filename string) bool {
	log.Debugln("Inside isJson", filename)
	if hasJsonExtension(filename) {
		log.Debugln(filename + " Has Json extension")
		content, err := ioutil.ReadFile(filename)
		if err != nil {
			log.Debugln("Unable to read file", filename)
			return false
		}
		// See if it's valid JSON
		var jsonData interface{}
		err = json.Unmarshal(content, &jsonData)
		if err != nil {
			log.Debugln("Unable to parse json. Error is", err)
			moveToDir(filename, viper.GetString("discard_dir"))
			log.Infoln("Moved " + filename + " to discard directory. Invalid JSON file or schema.")
			return false
		}
		return true
	}
	log.Debugln(filename + "Not a json file")
	return false
}
