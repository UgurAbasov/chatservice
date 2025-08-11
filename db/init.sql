-- Enable UUID generation
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";


-- Friendships table to track user relationships
CREATE TABLE friendships (
    user_one_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_two_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status VARCHAR(50) NOT NULL CHECK (status IN ('pending', 'accepted', 'blocked')),
    action_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_one_id, user_two_id),
    CONSTRAINT check_users_order CHECK (user_one_id < user_two_id) -- Ensures unique pairs
);

-- Rooms table for private and group chats
CREATE TABLE rooms (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    type VARCHAR(50) NOT NULL CHECK (type IN ('private', 'group')),
    name VARCHAR(255),
    owner_id UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Participants in a room
CREATE TABLE room_participants (
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(50) NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'admin', 'member')),
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    is_blocked BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (room_id, user_id)
);

-- Messages table
CREATE TABLE messages (
    id BIGSERIAL PRIMARY KEY,
    message_uid UUID UNIQUE NOT NULL DEFAULT uuid_generate_v4(),
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    reply_to_message_id BIGINT REFERENCES messages(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ
);

-- Message read status tracking
CREATE TABLE message_read_status (
    message_id BIGINT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    read_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (message_id, user_id)
);

-- Indexes for performance
CREATE INDEX ON users(email);
CREATE INDEX ON friendships(user_one_id, status);
CREATE INDEX ON friendships(user_two_id, status);
CREATE INDEX ON rooms(type);
CREATE INDEX ON room_participants(user_id);
CREATE INDEX ON messages(room_id, created_at DESC);
CREATE INDEX ON message_read_status(user_id);