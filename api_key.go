package main

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nlopes/slack"
	log "github.com/sirupsen/logrus"
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
	filename := ae.SlackId + ".json"
	err = os.WriteFile("./data/credentials/"+filename, jsonData, 0o644)
	if err != nil {
		log.Warnln("Error writing to file:", err)
		return
	}
	log.Debugln("JSON data written to", filename)
}

func validateApiKey() bool {
	log.Debugln("Inside validateApiKey")
	return false
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

func invalidateApiKey() bool {
	log.Debugln("Inside invalidateApiKey")
	return true
}

func apiEndpoint(c *gin.Context) {
	// startTime := time.Now()
	// Parse the form data, including files

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
