package websocket

import (
	"context"
	"log"

	"chatservice/internal/repository"
	"chatservice/internal/usecase"
	"chatservice/pkg/wprotocol"
	"github.com/google/uuid"
)

type PacketRequest struct { client *Client; data []byte }
type BroadcastMessage struct { RoomID uuid.UUID; Message []byte }
type DirectMessage struct { UserID uuid.UUID; Message []byte }
type SubscriptionRequest struct { ClientUserID uuid.UUID; RoomID uuid.UUID }

type Hub struct {
	clients     map[*Client]bool
	userClients map[uuid.UUID]*Client
	rooms       map[uuid.UUID]map[*Client]bool
	broadcast   chan *BroadcastMessage
	direct      chan *DirectMessage
	subscribe   chan *SubscriptionRequest
	process     chan *PacketRequest
	register    chan *Client
	unregister  chan *Client
	usecase     *usecase.AppUsecase
	repo        repository.AppRepository
}

func NewHub(repo repository.AppRepository) *Hub {
	return &Hub{
		clients:     make(map[*Client]bool),
		userClients: make(map[uuid.UUID]*Client),
		rooms:       make(map[uuid.UUID]map[*Client]bool),
		broadcast:   make(chan *BroadcastMessage, 256),
		direct:      make(chan *DirectMessage, 256),
		subscribe:   make(chan *SubscriptionRequest, 256),
		process:     make(chan *PacketRequest, 256),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		repo:        repo,
	}
}

func (h *Hub) SetUsecase(uc *usecase.AppUsecase) { h.usecase = uc }

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			h.userClients[client.userID] = client
			log.Printf("Client connected: %s", client.userID)
			userRooms, err := h.repo.GetRoomsForUser(context.Background(), client.userID)
			if err != nil { log.Printf("Error fetching rooms for user %s: %v", client.userID, err) } else {
				for _, room := range userRooms { h.doSubscribe(client, room.ID) }
			}

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				delete(h.userClients, client.userID)
				for roomID := range client.rooms { h.doUnsubscribe(client, roomID) }
				close(client.send)
				log.Printf("Client disconnected: %s", client.userID)
			}

		case req := <-h.process:
			packet, err := wprotocol.Parse(req.data)
			if err != nil { log.Printf("Error parsing packet from %s: %v", req.client.userID, err); continue }
			h.usecase.ProcessIncomingPacket(context.Background(), req.client.userID, packet)

		case broadcastMsg := <-h.broadcast:
			if roomClients, ok := h.rooms[broadcastMsg.RoomID]; ok {
				for client := range roomClients { client.sendMessage(broadcastMsg.Message) }
			}

		case directMsg := <-h.direct:
			if client, ok := h.userClients[directMsg.UserID]; ok {
				client.sendMessage(directMsg.Message)
			}

		case sub := <-h.subscribe:
			if client, ok := h.userClients[sub.ClientUserID]; ok {
				h.doSubscribe(client, sub.RoomID)
			}
		}
	}
}

func (h *Hub) doSubscribe(client *Client, roomID uuid.UUID) {
	if _, ok := h.rooms[roomID]; !ok { h.rooms[roomID] = make(map[*Client]bool) }
	h.rooms[roomID][client] = true
	client.rooms[roomID] = true
	log.Printf("Client %s subscribed to room %s", client.userID, roomID)
}

func (h *Hub) doUnsubscribe(client *Client, roomID uuid.UUID) {
	if room, ok := h.rooms[roomID]; ok {
		delete(room, client)
		if len(room) == 0 { delete(h.rooms, roomID) }
	}
	delete(client.rooms, roomID)
	log.Printf("Client %s unsubscribed from room %s", client.userID, roomID)
}

func (h *Hub) BroadcastToRoom(roomID uuid.UUID, message []byte) { h.broadcast <- &BroadcastMessage{RoomID: roomID, Message: message} }
func (h *Hub) SendToUser(userID uuid.UUID, message []byte) { h.direct <- &DirectMessage{UserID: userID, Message: message} }
func (h *Hub) Subscribe(clientUserID uuid.UUID, roomID uuid.UUID) { h.subscribe <- &SubscriptionRequest{ClientUserID: clientUserID, RoomID: roomID} }