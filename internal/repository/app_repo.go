package repository

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"chatservice/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AppRepository interface {
	UpsertUser(ctx context.Context, id uuid.UUID, email *string, nickname *string) error
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	CreateFriendship(ctx context.Context, fs *domain.Friendship) error
	UpdateFriendshipStatus(ctx context.Context, tx pgx.Tx, fs *domain.Friendship) error
	GetFriendship(ctx context.Context, userOneID, userTwoID uuid.UUID) (*domain.Friendship, error)
	GetFriendshipsForUser(ctx context.Context, userID uuid.UUID, status string) ([]domain.Friendship, error)
	DeleteFriendship(ctx context.Context, userOneID, userTwoID uuid.UUID) error
	IsUserInRoom(ctx context.Context, userID, roomID uuid.UUID) (bool, error)
	GetRoomByID(ctx context.Context, roomID uuid.UUID) (*domain.Room, error)
	CreateRoom(ctx context.Context, tx pgx.Tx, room *domain.Room) (*domain.Room, error)
	AddUserToRoom(ctx context.Context, tx pgx.Tx, userID, roomID uuid.UUID) error
	GetRoomsForUser(ctx context.Context, userID uuid.UUID) ([]domain.Room, error)
	GetMessagesForRoom(ctx context.Context, roomID uuid.UUID, limit, offset int) ([]domain.Message, error)
	CreateMessage(ctx context.Context, msg *domain.Message) (*domain.Message, error)
	MarkMessageAsRead(ctx context.Context, messageID int64, userID uuid.UUID) (*time.Time, error)
	FindPrivateRoomByParticipants(ctx context.Context, userOneID, userTwoID uuid.UUID) (uuid.UUID, error)
	SearchUsersByNickname(ctx context.Context, query string, selfID uuid.UUID, limit int) ([]domain.User, error)
	UpdateMessage(ctx context.Context, messageID int64, userID uuid.UUID, newContent string) error
	DeleteMessage(ctx context.Context, messageID int64, userID uuid.UUID) error	
}

type postgresAppRepository struct {
	db *pgxpool.Pool
}

func NewAppRepository(db *pgxpool.Pool) AppRepository {
	return &postgresAppRepository{db: db}
}

func (r *postgresAppRepository) UpsertUser(ctx context.Context, id uuid.UUID, email *string, nickname *string) error {	query := `INSERT INTO users (id, email) VALUES ($1, $2) ON CONFLICT (id) DO UPDATE SET email = COALESCE(users.email, $2)`
	_, err := r.db.Exec(ctx, query, id, email)
	return err
}

func (r *postgresAppRepository) UpdateMessage(ctx context.Context, messageID int64, userID uuid.UUID, newContent string) error {
	query := `
		UPDATE messages
		SET content = $1, updated_at = $2
		WHERE id = $3 AND user_id = $4
	`
	cmdTag, err := r.db.Exec(ctx, query, newContent, time.Now(), messageID, userID)
	if err != nil {
		return fmt.Errorf("error executing update message query: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("message not found or user not authorized to edit")
	}

	return nil
}

func (r *postgresAppRepository) DeleteMessage(ctx context.Context, messageID int64, userID uuid.UUID) error {
	query := `
		DELETE FROM messages
		WHERE id = $1 AND user_id = $2
	`
	cmdTag, err := r.db.Exec(ctx, query, messageID, userID)
	if err != nil {
		return fmt.Errorf("error executing delete message query: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("message not found or user not authorized to delete")
	}

	return nil
}

func (r *postgresAppRepository) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `SELECT id, email, nickname, username, created_at FROM users WHERE email = $1`
	rows, err := r.db.Query(ctx, query, email)
	if err != nil { return nil, err }
	user, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[domain.User])
	if errors.Is(err, pgx.ErrNoRows) { return nil, nil }
	return &user, err
}

func (r *postgresAppRepository) SearchUsersByNickname(ctx context.Context, query string, selfID uuid.UUID, limit int) ([]domain.User, error) {
	sqlQuery := `
		SELECT id, email, nickname, username, created_at 
		FROM users 
		WHERE nickname ILIKE $1 
		  AND id != $2
		LIMIT $3
	`
	
	rows, err := r.db.Query(ctx, sqlQuery, "%"+query+"%", selfID, limit)
	if err != nil {
		return nil, fmt.Errorf("error searching users: %w", err)
	}
	defer rows.Close()

	users, err := pgx.CollectRows(rows, pgx.RowToStructByName[domain.User])
	if err != nil {
		return nil, fmt.Errorf("error collecting user rows: %w", err)
	}

	return users, nil
}

func (r *postgresAppRepository) GetUserByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	query := `SELECT id, email, nickname, username, created_at FROM users WHERE id = $1`
	rows, err := r.db.Query(ctx, query, id)
	if err != nil { return nil, err }
	user, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[domain.User])
	if errors.Is(err, pgx.ErrNoRows) { return nil, nil }
	return &user, err
}

func (r *postgresAppRepository) FindPrivateRoomByParticipants(ctx context.Context, userOneID, userTwoID uuid.UUID) (uuid.UUID, error) {
	var roomID uuid.UUID
	query := `
		SELECT p1.room_id
		FROM room_participants p1
		JOIN room_participants p2 ON p1.room_id = p2.room_id
		JOIN rooms r ON p1.room_id = r.id
		WHERE r.type = 'private'
		  AND p1.user_id = $1
		  AND p2.user_id = $2
		  AND (
			  SELECT COUNT(*) 
			  FROM room_participants rp 
			  WHERE rp.room_id = p1.room_id
		  ) = 2
	`

	err := r.db.QueryRow(ctx, query, userOneID, userTwoID).Scan(&roomID)
	
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {

			return uuid.Nil, nil 
		}
		return uuid.Nil, fmt.Errorf("error finding private room: %w", err)
	}

	return roomID, nil
}

func (r *postgresAppRepository) CreateFriendship(ctx context.Context, fs *domain.Friendship) error {
	query := `INSERT INTO friendships (user_one_id, user_two_id, status, action_user_id) VALUES ($1, $2, $3, $4)`
	_, err := r.db.Exec(ctx, query, fs.UserOneID, fs.UserTwoID, fs.Status, fs.ActionUserID)
	return err
}

func (r *postgresAppRepository) UpdateFriendshipStatus(ctx context.Context, tx pgx.Tx, fs *domain.Friendship) error {
	query := `UPDATE friendships SET status = $3, action_user_id = $4, updated_at = NOW() WHERE user_one_id = $1 AND user_two_id = $2`
	_, err := tx.Exec(ctx, query, fs.UserOneID, fs.UserTwoID, fs.Status, fs.ActionUserID)
	return err
}

func (r *postgresAppRepository) GetFriendship(ctx context.Context, userOneID, userTwoID uuid.UUID) (*domain.Friendship, error) {
	if userOneID.String() > userTwoID.String() { userOneID, userTwoID = userTwoID, userOneID }
	query := `SELECT user_one_id, user_two_id, status, action_user_id, created_at, updated_at FROM friendships WHERE user_one_id = $1 AND user_two_id = $2`
	rows, err := r.db.Query(ctx, query, userOneID, userTwoID)
	if err != nil { return nil, err }
	fs, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[domain.Friendship])
	if errors.Is(err, pgx.ErrNoRows) { return nil, nil }
	return &fs, err
}

func (r *postgresAppRepository) GetFriendshipsForUser(ctx context.Context, userID uuid.UUID, status string) ([]domain.Friendship, error) {
	query := `SELECT user_one_id, user_two_id, status, action_user_id, created_at, updated_at FROM friendships WHERE (user_one_id = $1 OR user_two_id = $1) AND status = $2`
	rows, err := r.db.Query(ctx, query, userID, status)
	if err != nil { return nil, err }
	return pgx.CollectRows(rows, pgx.RowToStructByName[domain.Friendship])
}

func (r *postgresAppRepository) DeleteFriendship(ctx context.Context, userOneID, userTwoID uuid.UUID) error {
	if userOneID.String() > userTwoID.String() { userOneID, userTwoID = userTwoID, userOneID }
	query := `DELETE FROM friendships WHERE user_one_id = $1 AND user_two_id = $2`
	_, err := r.db.Exec(ctx, query, userOneID, userTwoID)
	return err
}

func (r *postgresAppRepository) IsUserInRoom(ctx context.Context, userID, roomID uuid.UUID) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM room_participants WHERE user_id = $1 AND room_id = $2 AND is_blocked = false)`
	err := r.db.QueryRow(ctx, query, userID, roomID).Scan(&exists)
	return exists, err
}

func (r *postgresAppRepository) GetRoomByID(ctx context.Context, roomID uuid.UUID) (*domain.Room, error) {
	query := `SELECT id, type, name, owner_id, created_at, updated_at FROM rooms WHERE id = $1`
	rows, err := r.db.Query(ctx, query, roomID)
	if err != nil { return nil, err }
	room, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[domain.Room])
	if errors.Is(err, pgx.ErrNoRows) { return nil, fmt.Errorf("room not found") }
	return &room, err
}

func (r *postgresAppRepository) CreateRoom(ctx context.Context, tx pgx.Tx, room *domain.Room) (*domain.Room, error) {
	query := `INSERT INTO rooms (type, name, owner_id) VALUES ($1, $2, $3) RETURNING id, created_at, updated_at`
	err := tx.QueryRow(ctx, query, room.Type, room.Name, room.OwnerID).Scan(&room.ID, &room.CreatedAt, &room.UpdatedAt)
	return room, err
}

func (r *postgresAppRepository) AddUserToRoom(ctx context.Context, tx pgx.Tx, userID, roomID uuid.UUID) error {
	query := `INSERT INTO room_participants (user_id, room_id) VALUES ($1, $2)`
	_, err := tx.Exec(ctx, query, userID, roomID)
	return err
}

func (r *postgresAppRepository) GetRoomsForUser(ctx context.Context, userID uuid.UUID) ([]domain.Room, error) {
	query := `
		WITH ranked_messages AS (
			SELECT 
				room_id,
				content,
				created_at,
				ROW_NUMBER() OVER(PARTITION BY room_id ORDER BY created_at DESC) as rn
			FROM messages
		)
		SELECT 
			r.id,
			r.type,
			r.name,
			lm.content as last_message_content,
			lm.created_at as last_message_created_at
		FROM 
			rooms r
		JOIN 
			room_participants rp ON r.id = rp.room_id
		LEFT JOIN 
			ranked_messages lm ON r.id = lm.room_id AND lm.rn = 1
		WHERE 
			rp.user_id = $1
		ORDER BY
			COALESCE(lm.created_at, r.created_at) DESC
	`
		rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("error getting rooms for user: %w", err)
	}
	defer rows.Close()

	var rooms []domain.Room
	for rows.Next() {
		var room domain.Room
		err := rows.Scan(
			&room.ID,
			&room.Type,
			&room.Name,
			&room.LastMessageContent,
			&room.LastMessageCreatedAt,
		)
		if err != nil {
			log.Printf("Warning: Error scanning room row: %v", err)
			continue 
		}
		rooms = append(rooms, room)
	}

	return rooms, nil
}

func (r *postgresAppRepository) GetMessagesForRoom(ctx context.Context, roomID uuid.UUID, limit, offset int) ([]domain.Message, error) {
	query := `SELECT id, message_uid, room_id, user_id, content, reply_to_message_id, created_at, updated_at, deleted_at FROM messages WHERE room_id = $1 AND deleted_at IS NULL ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	rows, err := r.db.Query(ctx, query, roomID, limit, offset)
	if err != nil { return nil, err }
	messages, err := pgx.CollectRows(rows, pgx.RowToStructByName[domain.Message])
	if err != nil { return nil, err }
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}

func (r *postgresAppRepository) CreateMessage(ctx context.Context, msg *domain.Message) (*domain.Message, error) {
	query := `INSERT INTO messages (message_uid, room_id, user_id, content, reply_to_message_id) VALUES (COALESCE($1, uuid_generate_v4()), $2, $3, $4, $5) RETURNING id, message_uid, created_at`
	err := r.db.QueryRow(ctx, query, msg.MessageUID, msg.RoomID, msg.UserID, msg.Content, msg.ReplyToMessageID).Scan(&msg.ID, &msg.MessageUID, &msg.CreatedAt)
	return msg, err
}

func (r *postgresAppRepository) MarkMessageAsRead(ctx context.Context, messageID int64, userID uuid.UUID) (*time.Time, error) {
	var readAt time.Time
	query := `INSERT INTO message_read_status (message_id, user_id, read_at) VALUES ($1, $2, NOW()) ON CONFLICT (message_id, user_id) DO UPDATE SET read_at = NOW() RETURNING read_at`
	err := r.db.QueryRow(ctx, query, messageID, userID).Scan(&readAt)
	return &readAt, err
}