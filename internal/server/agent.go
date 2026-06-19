package server

import (
	"bytes"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

const agentParseURL = "http://127.0.0.1:7501/parse"
const agentPostURL = "http://127.0.0.1:7501/post"

func ParseSMS(c *gin.Context) {
	proxyToAgent(c, agentParseURL)
}

func PostTransaction(c *gin.Context) {
	proxyToAgent(c, agentPostURL)
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
