package main

import (
	"fmt"
	"os"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"log"
)

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

	// Create a watcher

	var err error
	watcher, err = fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	// Use channels to coordinate goroutines
	done := make(chan struct{})

	go watchDirectory(directoryPath, done)
	go receiver(done)

	// Block and wait for a signal to exit
	<-done

}
