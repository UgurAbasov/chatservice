package http

import (
	"log"
	"net/http"
	"strconv"

	"chatservice/internal/middleware"
	"chatservice/internal/usecase"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AppHandler struct {
	uc usecase.AppUsecaseInterface
}



func NewAppHandler(uc usecase.AppUsecaseInterface) *AppHandler { 
	if uc == nil {
		log.Fatal("NewAppHandler received a nil usecase")
	}
	return &AppHandler{uc: uc}
}


func RegisterRoutes(api *gin.RouterGroup, uc usecase.AppUsecaseInterface) {
		h := NewAppHandler(uc)

	users := api.Group("/users")
	{
		users.POST("/me", h.updateUser)
		users.GET("/search", h.searchUsers)
	}

	friends := api.Group("/friends")
	{
		friends.GET("", h.getFriends)
		friends.POST("/requests", h.sendFriendRequest)
		friends.PUT("/requests/:requester_id/accept", h.acceptFriendRequest)
	}

	rooms := api.Group("/rooms")
	{
		rooms.GET("", h.getRooms)
		rooms.GET("/:id/messages", h.getMessages)
	}
}

type UpdateUserPayload struct {
	Email    *string `json:"email,omitempty"`
	Username *string `json:"username,omitempty"`
}

func (h *AppHandler) searchUsers(c *gin.Context) {
	selfID := c.MustGet(middleware.UserIDKey).(uuid.UUID)

	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "search query 'q' is required"})
		return
	}

	users, err := h.uc.SearchUsers(c.Request.Context(), query, selfID)
	if err != nil {
		log.Printf("Error from SearchUsers usecase: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to search for users"})
		return
	}

	c.JSON(http.StatusOK, users)
}

func (h *AppHandler) updateUser(c *gin.Context) {
	userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)
	var payload UpdateUserPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.uc.UpdateUser(c.Request.Context(), userID, payload.Email, payload.Username); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "user updated"})
}

type SendFriendRequestPayload struct {
	Email string `json:"email" binding:"required,email"`
}

func (h *AppHandler) getFriends(c *gin.Context) {
	userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)

	friendsList, err := h.uc.GetFriendsAndRequests(c.Request.Context(), userID)
	if err != nil {
		log.Printf("Error from GetFriendsAndRequests: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not fetch friends list"})
		return
	}

	c.JSON(http.StatusOK, friendsList)
}

func (h *AppHandler) sendFriendRequest(c *gin.Context) {
	senderID := c.MustGet(middleware.UserIDKey).(uuid.UUID)
	var payload SendFriendRequestPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.uc.SendFriendRequest(c.Request.Context(), senderID, payload.Email); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "friend request sent"})
}

func (h *AppHandler) acceptFriendRequest(c *gin.Context) {
	accepterID := c.MustGet(middleware.UserIDKey).(uuid.UUID)
	requesterID, err := uuid.Parse(c.Param("requester_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid requester ID"})
		return
	}
	if err := h.uc.AcceptFriendRequest(c.Request.Context(), accepterID, requesterID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "friend request accepted"})
}

func (h *AppHandler) getRooms(c *gin.Context) {
	userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)
	rooms, err := h.uc.GetRoomsForUser(c.Request.Context(), userID)
	if err != nil {
		log.Printf("Error from GetRoomsForUser: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not fetch rooms"})
		return
	}
	c.JSON(http.StatusOK, rooms)
}

func (h *AppHandler) getMessages(c *gin.Context) {
	userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)
	roomID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid room ID"})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	messages, err := h.uc.GetMessagesForRoom(c.Request.Context(), userID, roomID, limit, offset)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, messages)
}