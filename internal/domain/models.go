package domain

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID        uuid.UUID `json:"id" db:"id"`
	Email     string    `json:"email" db:"email"`
	Username  string    `json:"username" db:"username"`
	Nickname  string    `json:"nickname" db:"nickname"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
}

type Friendship struct {
	UserOneID    uuid.UUID `json:"user_one_id" db:"user_one_id"`
	UserTwoID    uuid.UUID `json:"user_two_id" db:"user_two_id"`
	Status       string    `json:"status" db:"status"`
	ActionUserID uuid.UUID `json:"action_user_id" db:"action_user_id"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

type Friend struct {
	ID       uuid.UUID `json:"id"`
	Nickname string    `json:"nickname"`
	RoomID   uuid.UUID `json:"roomId"`
}

type FriendRequest struct {
	SenderId   uuid.UUID `json:"senderId"`
	SenderName string    `json:"senderName"`
}


func NewFriendship(userOneID, userTwoID uuid.UUID, status string, actionUserID uuid.UUID) *Friendship {
	if userOneID.String() > userTwoID.String() {
		userOneID, userTwoID = userTwoID, userOneID
	}
	return &Friendship{
		UserOneID:    userOneID,
		UserTwoID:    userTwoID,
		Status:       status,
		ActionUserID: actionUserID,
	}
}

type Room struct {
	ID        uuid.UUID  `json:"id" db:"id"`
	Type      string     `json:"type" db:"type"`
	Name      *string    `json:"name,omitempty" db:"name"`
	OwnerID   *uuid.UUID `json:"owner_id,omitempty" db:"owner_id"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" db:"updated_at"`
	LastMessageContent    *string    `json:"lastMessageContent,omitempty" db:"last_message_content"`
	LastMessageCreatedAt *time.Time `json:"lastMessageCreatedAt,omitempty" db:"last_message_created_at"`
}

type Message struct {
	ID               int64      `json:"id" db:"id"`
	MessageUID       uuid.UUID  `json:"message_uid" db:"message_uid"`
	RoomID           uuid.UUID  `json:"room_id" db:"room_id"`
	UserID           uuid.UUID  `json:"user_id" db:"user_id"`
	Content          string     `json:"content" db:"content"`
	ReplyToMessageID *int64     `json:"reply_to_message_id,omitempty" db:"reply_to_message_id"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt        *time.Time `json:"updated_at,omitempty" db:"updated_at"`
	DeletedAt        *time.Time `json:"-" db:"deleted_at"`
}