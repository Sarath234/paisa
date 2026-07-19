package server

import (
	"bytes"
	"io"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
)

var agentBaseURL = "http://127.0.0.1:7501"

func ParseSMS(c *gin.Context) {
	proxyToAgent(c, agentBaseURL+"/parse")
}

func PostTransaction(c *gin.Context) {
	proxyToAgent(c, agentBaseURL+"/post")
}

func ChatWithAgent(c *gin.Context) {
	proxyToAgent(c, agentBaseURL+"/chat")
}

func proxyToAgent(c *gin.Context, url string) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "paisa-agent not reachable: " + err.Error()})
		return
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	c.Data(resp.StatusCode, "application/json", data)
}

// UploadStatement streams a multipart statement upload to the agent,
// preserving the multipart boundary.
func UploadStatement(c *gin.Context) {
	resp, err := http.Post(agentBaseURL+"/statement/upload",
		c.GetHeader("Content-Type"), c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "paisa-agent not reachable: " + err.Error()})
		return
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	c.Data(resp.StatusCode, "application/json", data)
}

// StatementStatus proxies the upload-status poll to the agent.
func StatementStatus(c *gin.Context) {
	resp, err := http.Get(agentBaseURL + "/statement/status?file=" + url.QueryEscape(c.Query("file")))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "paisa-agent not reachable: " + err.Error()})
		return
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	c.Data(resp.StatusCode, "application/json", data)
}
