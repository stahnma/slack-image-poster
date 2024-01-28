package main

import (
	"encoding/json"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/nlopes/slack"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"
)

// ImageInfo represents the data to be stored in the JSON file

const systemdUnitTemplate = `
[Unit]
Description={{.Description}}
After=network.target

[Service]
ExecStart={{.ExecStart}}
Restart=always
User={{.User}}

[Install]
WantedBy=multi-user.target
`

/*
export SLACK_TOKEN=xoxb-932576141793-945495155271-AavuLkyvFefCODOrdhFkeVHi
export IMAGE_DIRECTORY=`pwd`/images
export SLACK_CHANNEL="ai-junk"
*/

// verify the envionment variables are set
func verifyEnvVars() {
	requiredEnvVars := []string{"SLACK_TOKEN", "UPLOAD_DIR", "SLACK_CHANNEL", "UPLOAD_API_KEY"}
	for _, envVar := range requiredEnvVars {
		if os.Getenv(envVar) == "" {
			fmt.Println("Missing required environment variable: " + envVar)
			os.Exit(1)
		}
	}
}

// SystemdUnit represents the data for the systemd unit template
type SystemdUnit struct {
	Description string
	ExecStart   string
	User        string
}

func systemd_unit() {

	verifyEnvVars()
	// Define the data for the systemd unit template
	fq_program, err := os.Executable()
	if err != nil {
		panic(err)
	}
	current_user, _ := user.Current()

	unitData := SystemdUnit{
		Description: os.Args[0],
		ExecStart:   fq_program,
		User:        current_user.Username,
	}

	// Create a new template and parse the systemd unit template string
	tmpl, err := template.New("systemdUnit").Parse(systemdUnitTemplate)
	if err != nil {
		panic(err)
	}

	// Execute the template with the provided data and write to stdout
	err = tmpl.Execute(os.Stdout, unitData)
	if err != nil {
		panic(err)
	}
}

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

func main() {

	pflag.CommandLine.Usage = func() {
		fmt.Fprintf(os.Stderr, "Custom help text goes here.\n\n")
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", "your_app_name")
		pflag.PrintDefaults()
	}
	// Parse command line flags
	pflag.Parse()

	// Access the values using Viper
	configFile := viper.GetString("config")
	readySystemd := viper.GetBool("ready-systemd")

	// Load configuration file if specified
	if configFile != "" {
		viper.SetConfigFile(configFile)
		err := viper.ReadInConfig()
		if err != nil {
			fmt.Println("Error reading config file:", err)
		}
	}

	// Check if --ready-systemd flag is provided
	if readySystemd {
		systemd_unit()
		os.Exit(0)
	}

	// Get Slack API token, directory path, Slack channel from environment variables
	slackToken := os.Getenv("SLACK_TOKEN")
	directoryPath := os.Getenv("UPLOAD_DIR")
	slackChannel := os.Getenv("SLACK_CHANNEL")

	if slackToken == "" || directoryPath == "" || slackChannel == "" {
		fmt.Println("Please set SLACK_TOKEN, UPLOAD_DIR, and SLACK_CHANNEL environment variables.")
		os.Exit(1)
	}

	api := slack.New(slackToken)

	// Create a watcher
	watcher, _ = fsnotify.NewWatcher()
	defer watcher.Close()

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

		api := slack.New(os.Getenv("SLACK_TOKEN"))

		// Upload the new image to Slack
		err := uploadImageToSlack(api, filePath, os.Getenv("SLACK_CHANNEL"))
		if err != nil { fmt.Printf("File %s not uploaded. Error: %v\n", filepath.Base(filePath), err)
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
				fmt.Printf("File %s uploaded to Slack and moved to processed folder.\n", file.Name())
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
		File:           filePath, // Use file path here
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

type ImageInfo struct {
	ImagePath string `json:"image_path"`
	Caption   string `json:"caption"`
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	// Check for the presence of the X-API-Key header
	apiKey := r.Header.Get("X-API-Key")
	expectedKey := os.Getenv("UPLOAD_API_KEY") // Read API key from environment variable

	if apiKey != expectedKey {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse the form data, including files
	err := r.ParseMultipartForm(10 << 20) // 10 MB limit
	if err != nil {
		http.Error(w, "Unable to parse form", http.StatusBadRequest)
		return
	}

	// Get the image file and other form data
	image, imageHeader, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "Unable to get image", http.StatusBadRequest)
		return
	}
	defer image.Close()

	caption := r.FormValue("caption")

	// Create a unique filename for the uploaded image
	imageName := fmt.Sprintf("%d_%s", getCurrentTimestamp(), imageHeader.Filename)
	uploadDir := os.Getenv("UPLOAD_DIR")
	imagePath := filepath.Join(uploadDir, imageName)
	file, err := os.Create(imagePath)
	if err != nil {
		http.Error(w, "Unable to create file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Save the image to disk
	_, err = io.Copy(file, image)
	if err != nil {
		http.Error(w, "Unable to save image", http.StatusInternalServerError)
		return
	}

	// Create and save the JSON file
	jsonPath := filepath.Join(uploadDir, fmt.Sprintf("%s.json", imageName))
	imageInfo := ImageInfo{ImagePath: imagePath, Caption: caption}
	jsonData, err := json.MarshalIndent(imageInfo, "", "    ")
	if err != nil {
		http.Error(w, "Unable to create JSON", http.StatusInternalServerError)
		return
	}
	err = os.WriteFile(jsonPath, jsonData, 0644)
	if err != nil {
		http.Error(w, "Unable to save JSON", http.StatusInternalServerError)
		return
	}

	// Respond with success message
	w.Write([]byte("Upload successful"))
}

func getCurrentTimestamp() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

func recever() {
	// Read environment variables
	uploadDir := os.Getenv("UPLOAD_DIR")
	apiKey := os.Getenv("UPLOAD_API_KEY")

	// Validate environment variables
	if uploadDir == "" {
		fmt.Println("UPLOAD_DIR environment variable not set.")
		os.Exit(1)
	}

	if apiKey == "" {
		fmt.Println("UPLOAD_API_KEY environment variable not set.")
		os.Exit(1)
	}

	// declare port as an int
	p := ""
	if value, ok := os.LookupEnv("UPLOAD_PORT"); ok {
		p = value
	} else {
		p = "8080"
	}
	port, err := strconv.Atoi(p)
	if err != nil {
		panic(err)
	}

	// Create upload directory if it doesn't exist
	if _, err := os.Stat(uploadDir); os.IsNotExist(err) {
		os.Mkdir(uploadDir, 0755)
	}

	// Set up the HTTP server
	http.HandleFunc("/upload", uploadHandler)
	serverAddr := fmt.Sprintf(":%d", port)

	// Start the server
	fmt.Printf("Server listening on port %d...\n", port)
	err = http.ListenAndServe(serverAddr, nil)
	if err != nil {
		fmt.Println("Error:", err)
	}
}
