package server

import (
	"net/http"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/ananthakumaran/paisa/internal/service"
	"github.com/gin-gonic/gin"
)

type pushSubscribeRequest struct {
	Endpoint string `json:"endpoint" binding:"required"`
	Keys     struct {
		Auth   string `json:"auth" binding:"required"`
		P256dh string `json:"p256dh" binding:"required"`
	} `json:"keys" binding:"required"`
}

func GetPushPublicKey(publicKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"publicKey": publicKey})
	}
}

func PostPushSubscribe(dir, publicKey, privateKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req pushSubscribeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		sub := &webpush.Subscription{
			Endpoint: req.Endpoint,
			Keys: webpush.Keys{
				Auth:   req.Keys.Auth,
				P256dh: req.Keys.P256dh,
			},
		}

		if err := service.SaveSubscription(dir, sub); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save subscription"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}
