package usecase

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"chatservice/internal/domain"
	"chatservice/internal/repository"
	"chatservice/pkg/wprotocol"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FriendsList struct {
	Friends  []domain.Friend  `json:"friends"`
	Requests []domain.FriendRequest `json:"requests"`
}


type AppUsecaseInterface interface {
	UpdateUser(ctx context.Context, id uuid.UUID, email *string, nickname *string) error
	SendFriendRequest(ctx context.Context, senderID uuid.UUID, receiverEmail string) error
	AcceptFriendRequest(ctx context.Context, accepterID, requesterID uuid.UUID) error
	GetRoomsForUser(ctx context.Context, userID uuid.UUID) ([]domain.Room, error)
	GetMessagesForRoom(ctx context.Context, userID, roomID uuid.UUID, limit, offset int) ([]domain.Message, error)
	ProcessIncomingPacket(ctx context.Context, senderID uuid.UUID, packet *wprotocol.Packet)
	GetFriendsAndRequests(ctx context.Context, userID uuid.UUID) (*FriendsList, error)
	SearchUsers(ctx context.Context, query string, selfID uuid.UUID) ([]domain.User, error)
}

type Broadcaster interface {
	BroadcastToRoom(roomID uuid.UUID, message []byte)
	SendToUser(userID uuid.UUID, message []byte)
	Subscribe(clientUserID uuid.UUID, roomID uuid.UUID)
}

type AppUsecase struct {
	repo  repository.AppRepository
	bcast Broadcaster
	db    *pgxpool.Pool 
}

func NewAppUsecase(repo repository.AppRepository, bcast Broadcaster, db *pgxpool.Pool) AppUsecaseInterface {
	return &AppUsecase{
		repo:  repo,
		bcast: bcast,
		db:    db,
	}
}



func (uc *AppUsecase) UpdateUser(ctx context.Context, id uuid.UUID, email *string, nickname *string) error {
	return uc.repo.UpsertUser(ctx, id, email, nickname)
}


func (uc *AppUsecase) GetFriendsAndRequests(ctx context.Context, userID uuid.UUID) (*FriendsList, error) {
	acceptedFriendships, err := uc.repo.GetFriendshipsForUser(ctx, userID, "accepted")
	if err != nil {
		return nil, fmt.Errorf("could not fetch friends: %w", err)
	}

	pendingFriendships, err := uc.repo.GetFriendshipsForUser(ctx, userID, "pending")
	if err != nil {
		return nil, fmt.Errorf("could not fetch friend requests: %w", err)
	}

	response := &FriendsList{
		Friends:  []domain.Friend{},
		Requests: []domain.FriendRequest{},
	}

	for _, fs := range acceptedFriendships {
		var friendID uuid.UUID
		if fs.UserOneID == userID {
			friendID = fs.UserTwoID
		} else {
			friendID = fs.UserOneID
		}

		friendUser, err := uc.repo.GetUserByID(ctx, friendID)
		if err != nil || friendUser == nil {
			log.Printf("Warning: could not find user data for friend ID %s", friendID)
			continue
		}
		
		sharedRoomID, err := uc.repo.FindPrivateRoomByParticipants(ctx, userID, friendID)
		if err != nil {
			log.Printf("Error finding shared room for users %s and %s: %v", userID, friendID, err)
		}
		if sharedRoomID == uuid.Nil {
			log.Printf("Warning: no shared private room found for users %s and %s", userID, friendID)
		}
		
		response.Friends = append(response.Friends, domain.Friend{
			ID:       friendUser.ID,
			Nickname: friendUser.Nickname,
			RoomID:   sharedRoomID, 
		})
	}

	for _, fs := range pendingFriendships {
		if fs.ActionUserID != userID {
			requesterID := fs.ActionUserID
			
			requester, err := uc.repo.GetUserByID(ctx, requesterID)
			if err != nil || requester == nil {
				log.Printf("Warning: could not find user data for requester ID %s", requesterID)
				continue
			}
		

			response.Requests = append(response.Requests, domain.FriendRequest{
				SenderId:   requester.ID,
				SenderName: requester.Nickname,
			})
		}
	}

	return response, nil
}

func (uc *AppUsecase) SearchUsers(ctx context.Context, query string, selfID uuid.UUID) ([]domain.User, error) {
	if len(query) < 2 {
		return []domain.User{}, nil 
	}
	return uc.repo.SearchUsersByNickname(ctx, query, selfID, 10)
}

func (uc *AppUsecase) SendFriendRequest(ctx context.Context, senderID uuid.UUID, receiverEmail string) error {	sender, err := uc.repo.GetUserByID(ctx, senderID)
	if err != nil || sender == nil {
		return fmt.Errorf("sender not found")
	}

	receiver, err := uc.repo.GetUserByEmail(ctx, receiverEmail)
	if err != nil || receiver == nil {
		return fmt.Errorf("user with email %s not found", receiverEmail)
	}

	if senderID == receiver.ID {
		return fmt.Errorf("cannot send friend request to yourself")
	}

	existingFs, err := uc.repo.GetFriendship(ctx, senderID, receiver.ID)
	if err != nil {
		return fmt.Errorf("error checking existing friendship: %w", err)
	}

	if existingFs != nil {
		return fmt.Errorf("a friendship or pending request already exists with this user")
	}

	fs := domain.NewFriendship(senderID, receiver.ID, "pending", senderID)
	if err := uc.repo.CreateFriendship(ctx, fs); err != nil {
		return fmt.Errorf("failed to create friend request: %w", err)
	}

	senderName := sender.Nickname

	notification := wprotocol.Build(wprotocol.OpFriendRequestReceived, senderID.String(), senderName)
	uc.bcast.SendToUser(receiver.ID, notification)

	log.Printf("User %s sent friend request to user %s", senderID, receiver.ID)
	return nil
}

func (uc *AppUsecase) AcceptFriendRequest(ctx context.Context, accepterID, requesterID uuid.UUID) error {
	fs, err := uc.repo.GetFriendship(ctx, accepterID, requesterID)
	if err != nil || fs == nil {
		return fmt.Errorf("no pending friend request found")
	}
	if fs.Status != "pending" || fs.ActionUserID == accepterID {
		return fmt.Errorf("invalid friend request state")
	}

	tx, err := uc.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("could not begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) 

	fs.Status = "accepted"
	fs.ActionUserID = accepterID
	if err := uc.repo.UpdateFriendshipStatus(ctx, tx, fs); err != nil {
		return fmt.Errorf("failed to update friendship: %w", err)
	}

	room := &domain.Room{Type: "private"}
	createdRoom, err := uc.repo.CreateRoom(ctx, tx, room)
	if err != nil {
		return fmt.Errorf("failed to create private room: %w", err)
	}

	if err := uc.repo.AddUserToRoom(ctx, tx, accepterID, createdRoom.ID); err != nil {
		return fmt.Errorf("failed to add accepter to room: %w", err)
	}
	if err := uc.repo.AddUserToRoom(ctx, tx, requesterID, createdRoom.ID); err != nil {
		return fmt.Errorf("failed to add requester to room: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("transaction commit failed: %w", err)
	}

	accepter, _ := uc.repo.GetUserByID(ctx, accepterID)
	accepterName := accepter.Nickname

	notificationToRequester := wprotocol.Build(
		wprotocol.OpFriendRequestAccepted,
		accepterID.String(),
		accepterName,
		createdRoom.ID.String(),
	)
	uc.bcast.SendToUser(requesterID, notificationToRequester)
	uc.bcast.Subscribe(requesterID, createdRoom.ID) 

	notificationToAccepter := wprotocol.Build(
		wprotocol.OpNotifyRoomAdded,
		createdRoom.ID.String(),
		createdRoom.Type,
		"",
	)
	uc.bcast.SendToUser(accepterID, notificationToAccepter)
	uc.bcast.Subscribe(accepterID, createdRoom.ID)

	log.Printf("User %s accepted friend request from %s. Private room %s created.", accepterID, requesterID, createdRoom.ID)
	return nil
}


func (uc *AppUsecase) GetRoomsForUser(ctx context.Context, userID uuid.UUID) ([]domain.Room, error) {
	return uc.repo.GetRoomsForUser(ctx, userID)
}

func (uc *AppUsecase) GetMessagesForRoom(ctx context.Context, userID, roomID uuid.UUID, limit, offset int) ([]domain.Message, error) {
	isMember, err := uc.repo.IsUserInRoom(ctx, userID, roomID)
	if err != nil {
		return nil, fmt.Errorf("could not verify room membership: %w", err)
	}
	if !isMember {
		return nil, fmt.Errorf("user not authorized to access this room")
	}
	return uc.repo.GetMessagesForRoom(ctx, roomID, limit, offset)
}

func (uc *AppUsecase) ProcessIncomingPacket(ctx context.Context, senderID uuid.UUID, packet *wprotocol.Packet) {
	checkMembership := func(roomID uuid.UUID) bool {
		isMember, err := uc.repo.IsUserInRoom(ctx, senderID, roomID)
		if err != nil {
			log.Printf("Error checking membership for user %s in room %s: %v", senderID, roomID, err)
			return false
		}
		if !isMember {
			log.Printf("AuthZ Error: User %s not in room %s", senderID, roomID)
			uc.bcast.SendToUser(senderID, wprotocol.Build(wprotocol.OpError, "Not a member of this room"))
			return false
		}
		return true
	}

	switch packet.Op {
	case wprotocol.OpMsgSend:
		if len(packet.Payload) < 3 { return }
		roomID, _ := uuid.Parse(packet.Payload[0])
		clientMsgUID, _ := uuid.Parse(packet.Payload[1])
		content := packet.Payload[2]
		
		if !checkMembership(roomID) { return }
		uc.handleSendMessage(ctx, senderID, roomID, clientMsgUID, content)

	case wprotocol.OpMsgEdit:
		if len(packet.Payload) < 3 { return }
		msgID, err := strconv.ParseInt(packet.Payload[0], 10, 64)
		if err != nil { return }
		roomID, err := uuid.Parse(packet.Payload[1])
		if err != nil { return }
		newContent := packet.Payload[2]
		
		if !checkMembership(roomID) { return }
		uc.handleEditMessage(ctx, senderID, msgID, roomID, newContent)

	case wprotocol.OpMsgDelete:
		if len(packet.Payload) < 2 { return }
		msgID, err := strconv.ParseInt(packet.Payload[0], 10, 64)
		if err != nil { return }
		roomID, err := uuid.Parse(packet.Payload[1])
		if err != nil { return }

		if !checkMembership(roomID) { return }
		uc.handleDeleteMessage(ctx, senderID, msgID, roomID)

	case wprotocol.OpMsgRead:
		if len(packet.Payload) < 2 { return }
		msgID, _ := strconv.ParseInt(packet.Payload[0], 10, 64)
		roomID, _ := uuid.Parse(packet.Payload[1])
		if !checkMembership(roomID) { return }
		uc.handleReadMessage(ctx, msgID, senderID, roomID)

    case wprotocol.OpWebRTCSignal:
		    if len(packet.Payload) < 2 {
        log.Printf("Invalid WebRTC signal packet from %s: insufficient payload", senderID)
        return
    }

    roomIDStr := packet.Payload[0]
    roomID, err := uuid.Parse(roomIDStr)
    if err != nil {
        log.Printf("Invalid roomID in WebRTC signal from %s: %v", senderID, err)
        return
    }

    signalDataString := packet.Payload[1]

    isMember, err := uc.repo.IsUserInRoom(ctx, senderID, roomID)
    if err != nil || !isMember {
        log.Printf("AuthZ Error: User %s tried to send signal to room %s without being a member", senderID, roomID)
        return
    }

    forwardPacket := wprotocol.Build(
        wprotocol.OpWebRTCSignal,
        senderID.String(), 
        roomIDStr,         
        signalDataString,  
    )

    uc.bcast.BroadcastToRoom(roomID, forwardPacket)
	default:
		log.Printf("Unknown or unhandled opcode received: %d", packet.Op)
	}
}

func (uc *AppUsecase) handleEditMessage(ctx context.Context, senderID uuid.UUID, msgID int64, roomID uuid.UUID, newContent string) {
	err := uc.repo.UpdateMessage(ctx, msgID, senderID, newContent)
	if err != nil {
		log.Printf("Failed to edit message %d by user %s: %v", msgID, senderID, err)
		uc.bcast.SendToUser(senderID, wprotocol.Build(wprotocol.OpError, "Failed to edit message"))
		return
	}

	msg := wprotocol.Build(
		wprotocol.OpMsgEdited,
		strconv.FormatInt(msgID, 10),
		roomID.String(),
		newContent,
	)
	uc.bcast.BroadcastToRoom(roomID, msg)
	log.Printf("User %s edited message %d in room %s", senderID, msgID, roomID)
}


func (uc *AppUsecase) handleDeleteMessage(ctx context.Context, senderID uuid.UUID, msgID int64, roomID uuid.UUID) {
	err := uc.repo.DeleteMessage(ctx, msgID, senderID)
	if err != nil {
		log.Printf("Failed to delete message %d by user %s: %v", msgID, senderID, err)
		uc.bcast.SendToUser(senderID, wprotocol.Build(wprotocol.OpError, "Failed to delete message"))
		return
	}

	msg := wprotocol.Build(
		wprotocol.OpMsgDeleted,
		strconv.FormatInt(msgID, 10),
		roomID.String(),
	)
	uc.bcast.BroadcastToRoom(roomID, msg)
	log.Printf("User %s deleted message %d in room %s", senderID, msgID, roomID)
}


func (uc *AppUsecase) handleSendMessage(ctx context.Context, senderID, roomID, clientMsgUID uuid.UUID, content string) {
	dbMsg := &domain.Message{
		MessageUID: clientMsgUID,
		RoomID:     roomID,
		UserID:     senderID,
		Content:    content,
	}

	createdMsg, err := uc.repo.CreateMessage(ctx, dbMsg)
	if err != nil {
		log.Printf("Failed to save message: %v", err)
		return
	}

	msg := wprotocol.Build(
		wprotocol.OpMsgDeliver,
		strconv.FormatInt(createdMsg.ID, 10),
		createdMsg.MessageUID.String(),
		createdMsg.RoomID.String(),
		createdMsg.UserID.String(),
		createdMsg.CreatedAt.Format(time.RFC3339Nano),
		createdMsg.Content,
	)
	uc.bcast.BroadcastToRoom(roomID, msg)
}

func (uc *AppUsecase) handleReadMessage(ctx context.Context, msgID int64, userID, roomID uuid.UUID) {
	readAt, err := uc.repo.MarkMessageAsRead(ctx, msgID, userID)
	if err != nil {
		log.Printf("Failed to mark message as read: %v", err)
		return
	}

	msg := wprotocol.Build(
		wprotocol.OpMsgStatusUpdate,
		strconv.FormatInt(msgID, 10),
		roomID.String(),
		userID.String(),
		"read",
		readAt.Format(time.RFC3339Nano),
	)
	uc.bcast.BroadcastToRoom(roomID, msg)
}