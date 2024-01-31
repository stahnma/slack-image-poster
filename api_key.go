package main

import (
	"encoding/json"
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

func (ae ApiEntry) save() {
	log.Debugln("Inside save")
	// write the entry to JSON
	jsonData, err := json.Marshal(ae)
	if err != nil {
		log.Errorln("Error:", err)
		return
	}
	filename := ae.SlackId + ".json"
	err = os.WriteFile(filename, jsonData, 0o644)
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

func getAuthor() string {

	log.Debugln("Inside getAuthor")

	return ""
}

func invalidateApiKey() bool {
	log.Debugln("Inside invalidateApiKey")
	return true
}

func apiEndpoint(c *gin.Context) {

}

// TODO get team id from a global var
func issueNewApiKey(slackId string) {
	// get the input from POST
	log.Debugln("Inside issueNewApiKey")
	// listen on /api
	// if  you provide a slack UUID, we deliver an API key.
	// The reason we don't just use the slack UUID is that anybody else could spoof you
	// stahnma = DTEGY4QDP
	// slackteam = TTEGY45PB

	var keyBlob ApiEntry
	// b, err := validateSlackId("DTEGY4QDP", "TTEGY45PB")
	b, err := validateSlackId(slackId, "TTEGY45PB")
	if err != nil {
		log.Errorln("Error validating slack id: ", err)
	}
	// at this point we know the slack id is valid
	if b {
		keyBlob.IssueDate = time.Now().String()
		keyBlob.ApiKey = generateApiKey()
		keyBlob.SlackId = slackId
		// TODO creatre revocation mechanism
		keyBlob.Revoked = false
	}

	// write api key to json file
	// display it for http response
	// generate new API key
}

func generateApiKey() string {
	log.Debugln("Inside generateApiKey")
	key := uuid.NewString()
	log.Debugln("Generated key: ", key)
	return key
}

func validateSlackId(userID, teamID string) (bool, error) {
	//FIXME: Load this into a global state
	token := os.Getenv("SLACK_TOKEN")
	log.Debugln("Inside validateSlackId")
	api := slack.New(token)

	// Get user info
	userInfo, err := api.GetUserInfo(userID)
	if err != nil {
		log.Infoln("UserId " + userID + " not found in team " + teamID)
		return false, err
	}

	// Check if the user is in the specified team
	return userInfo.TeamID == teamID, nil
}
