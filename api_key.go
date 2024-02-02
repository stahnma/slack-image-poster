package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nlopes/slack"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type ApiEntry struct {
	ApiKey    string `json:"api_key"`
	IssueDate string `json:"issue_date"`
	LastUsed  string `json:"last_used"`
	Revoked   bool   `json:"revoked"`
	SlackId   string `json:"slack_id"`
}

type ApiKeyRequest struct {
	SlackId string `json:"slack_id"`
}

func (ae ApiEntry) save() {
	log.Debugln("Inside save")
	jsonData, err := json.Marshal(ae)
	if err != nil {
		log.Errorln("Error:", err)
		return
	}
	filename := ae.ApiKey + ".json"
	err = os.WriteFile(viper.GetString("credentials_dir")+"/"+filename, jsonData, 0o644)
	if err != nil {
		log.Warnln("Error writing to file:", err)
		return
	}
	log.Debugln("JSON data written to", filename)
}

func (ae ApiEntry) isRevoked() bool {
	return ae.Revoked
}

func SearchAPIKeyInDirectory(searchString string) ([]string, error) {
	directoryPath := viper.GetString("credentials_dir")
	var matches []string
	err := filepath.Walk(directoryPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".json") {
			if match, err := searchAPIKeyInFile(path, searchString); err == nil && match {
				matches = append(matches, path)
			}
		}
		return nil
	})
	return matches, err
}

func searchAPIKeyInFile(filePath, searchString string) (bool, error) {
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		return false, err
	}
	var ae ApiEntry
	err = json.Unmarshal(fileContent, &ae)
	if err != nil {
		return false, err
	}
	return ae.ApiKey == searchString, nil
}

// Fixme this is not quite right yet
func validateApiKey(key string) (bool, error) {
	log.Debugln("Inside validateApiKey")
	// give a key, scan all files  and look for it
	matches, err := SearchAPIKeyInDirectory(key)
	if err != nil {
		log.Errorln("Error:", err)
		return false, nil
	}
	log.Debugln("Matches", matches)
	log.Debugln("Lenght of Matches", len(matches))
	if len(matches) < 1 {
		return false, nil
	}
	for _, match := range matches {
		log.Debugln("Match found in file:", match)
		isRev, err := isRevoked(match)
		if err != nil {
			log.Errorln("Error:", err)
			return false, nil
		}
		revErr := errors.New("Key has been revoked")
		if isRev {
			log.Debugln("Key has been revoked")
			return false, revErr
		}
	}
	return true, nil
}

func loadApiEntryFromFile(filePath string) (ApiEntry, error) {
	log.Debugln("Inside loadApiEntryFromFile", filePath)
	var ae ApiEntry
	filecontent, err := os.ReadFile(filePath)
	if err != nil {
		log.Errorln("(loadApiEntryFromFile) Error reading file:"+filePath+" ", err)
		return ae, err
	}
	err = json.Unmarshal(filecontent, &ae)
	if err != nil {
		log.Errorln("(loadApiEntryFromFile) Error unmarshalling json from keyfile ", filePath, err)
		return ae, err
	}
	return ae, nil
}

// FIXME what needs this?

/*func getAuthor() string {
	log.Debugln("Inside getAuthor")
	//FIXME: Load this into a global state
	token := os.Getenv("SLACK_TOKEN")
	log.Debugln("Inside validateSlackId. userId: ", userID, " teamId: ", teamID, " token: ", token)
	api := slack.New(token)
	userInfo, err := api.GetUserInfo(userID)
	log.Debugln("userInfo", userInfo)
	if err != nil {
		log.Infoln("UserId " + userID + " not found in team " + teamID)
		return ""
	}
	return ""
	//return userInfo.TeamID == teamID

}
*/

// TODO move this http handler to a separate file
func apiEndpoint(c *gin.Context) {
	// startTime := time.Now()
	// Parse the form data, including files
	// IF DELETE, then invalidate the key

	apiKey := c.GetHeader("X-API-Key")

	// DELETE
	if c.Request.Method == "DELETE" {
		log.Debugln("DELETE request")
		good, err := validateApiKey(apiKey)
		if err != nil {
			log.Warnln("Error:", err)
			c.JSON(http.StatusNetworkAuthenticationRequired, gin.H{"status": "error", "message": err.Error()})
		}
		if good {
			revoked := revokeApiKey(apiKey)
			if revoked {
				c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "Key revoked."})
			} else {
				log.Errorln("Unable to revoke key, but key file found. This is bad.")
				c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Unable to revoke key."})
			}
		} else {
			log.Warnln("Unalbe to revoke key because key not valid.", apiKey)
			c.JSON(http.StatusNetworkAuthenticationRequired, gin.H{"status": "error", "message": "Key not valid."})
		}
	}

	// POST
	if c.Request.Method == "POST" {
		var ae ApiKeyRequest
		if err := c.ShouldBindJSON(&ae); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			log.Debugln("Error processing JSON POST")
			return
		}
		log.Debugln("ae.SlackId is:", ae.SlackId)
		slackId := ae.SlackId
		if apikey := issueNewApiKey(slackId); apikey != "" {
			c.JSON(http.StatusOK, gin.H{"status": "ok", "apikey": apikey})
		} else {
			c.JSON(http.StatusNetworkAuthenticationRequired, gin.H{"status": "SlackID not found for team."})
		}
	}

	// TODO do a re-issue in a single operation
}

// TODO get team id from a global var
func issueNewApiKey(slackId string) string {
	log.Debugln("Inside issueNewApiKey. slackId", slackId)
	var keyBlob ApiEntry
	b := validateSlackId(slackId, "TTEGY45PB")
	log.Debugln("validateSlackId returned: ", b)
	// at this point we know the slack id is valid
	if b {
		keyBlob.IssueDate = time.Now().String()
		keyBlob.ApiKey = generateApiKey()
		keyBlob.SlackId = slackId
		// TODO creatre revocation mechanism
		keyBlob.Revoked = false
		log.Debugln("keyBlob: ", keyBlob.ApiKey)
		keyBlob.save()
		return keyBlob.ApiKey
	}
	return ""
}

func revokeApiKey(key string) bool {
	log.Debugln("Inside revokeApiKey", key)
	// find the key file
	// change json to revoked = true
	var ae ApiEntry
	keyfile := viper.GetString("credentials_dir") + "/" + key + ".json"
	filecontent, err := os.ReadFile(keyfile)
	if err != nil {
		log.Errorln("(revokeAPiKey) Error reading file:"+keyfile+" ", err)
		return false
	}
	// read file into json struct
	err = json.Unmarshal(filecontent, &ae)
	if err != nil {
		log.Errorln("Error unmarshalling json from keyfile ", keyfile, err)
		return false
	}
	ae.Revoked = true
	ae.save()
	return true
}

// Fixme implement
func isRevoked(filePath string) (bool, error) {
	log.Debugln("Inside isRevoked", filePath)
	ae, err := loadApiEntryFromFile(filePath)
	if err != nil {
		log.Errorln("Error loading api entry from file", filePath, err)
		// This is fail-safe
		return true, err
	}
	log.Debugln("ae.Revoked", ae.Revoked)
	return ae.Revoked == true, nil
}

func generateApiKey() string {
	log.Debugln("Inside generateApiKey")
	key := uuid.NewString()
	log.Debugln("Generated key: ", key)
	return key
}

func validateSlackId(userID, teamID string) bool {
	//FIXME: Load this into a global state
	token := os.Getenv("SLACK_TOKEN")
	log.Debugln("Inside validateSlackId. userId: ", userID, " teamId: ", teamID, " token: ", token)
	api := slack.New(token)
	userInfo, err := api.GetUserInfo(userID)
	log.Debugln("userInfo.TeamID", userInfo.TeamID)
	if err != nil {
		log.Infoln("UserId " + userID + " not found in team " + teamID)
		return false
	}
	return userInfo.TeamID == teamID
}
