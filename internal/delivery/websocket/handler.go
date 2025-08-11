package websocket

import (
	"log"
	"net/http"

	"chatservice/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func ServeWs(hub *Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)

		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Println(err)
			return
		}

		client := &Client{
			hub:    hub,
			conn:   conn,
			send:   make(chan []byte, 256),
			userID: userID,
			rooms:  make(map[uuid.UUID]bool),
		}
		client.hub.register <- client

		go client.writePump()
		go client.readPump()
	}
}