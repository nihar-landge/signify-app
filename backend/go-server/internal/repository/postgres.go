package repository

import (
	"context"
	"database/sql"
	"log"
	"time"

	_ "github.com/lib/pq"
)

// PostgresRepository handles PostgreSQL database operations
type PostgresRepository struct {
	db *sql.DB
}

// NewPostgresRepository creates a new PostgreSQL repository
func NewPostgresRepository(databaseURL string) (*PostgresRepository, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}

	log.Println("Connected to PostgreSQL database")

	repo := &PostgresRepository{db: db}

	// Run migrations
	if err := repo.migrate(); err != nil {
		return nil, err
	}

	return repo, nil
}

// migrate creates the database schema
func (r *PostgresRepository) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id VARCHAR(255) PRIMARY KEY,
		name VARCHAR(255),
		email VARCHAR(255),
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS conversations (
		id VARCHAR(255) PRIMARY KEY,
		user_id VARCHAR(255) REFERENCES users(id),
		title VARCHAR(255),
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS messages (
		id VARCHAR(255) PRIMARY KEY,
		conversation_id VARCHAR(255) REFERENCES conversations(id),
		role VARCHAR(50),
		content TEXT,
		content_type VARCHAR(50) DEFAULT 'text',
		confidence FLOAT,
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_conversations_user_id ON conversations(user_id);
	CREATE INDEX IF NOT EXISTS idx_messages_conversation_id ON messages(conversation_id);
	`

	_, err := r.db.Exec(schema)
	if err != nil {
		log.Printf("Migration error: %v", err)
		return err
	}

	log.Println("Database migrations completed")
	return nil
}

// SaveConversation saves a new conversation
func (r *PostgresRepository) SaveConversation(userID, conversationID, title string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		INSERT INTO conversations (id, user_id, title)
		VALUES ($1, $2, $3)
		ON CONFLICT (id) DO UPDATE SET updated_at = CURRENT_TIMESTAMP
	`

	_, err := r.db.ExecContext(ctx, query, conversationID, userID, title)
	return err
}

// SaveMessage saves a message to a conversation
func (r *PostgresRepository) SaveMessage(conversationID, messageID, role, content, contentType string, confidence float32) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		INSERT INTO messages (id, conversation_id, role, content, content_type, confidence)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	_, err := r.db.ExecContext(ctx, query, messageID, conversationID, role, content, contentType, confidence)
	return err
}

// GetConversations returns all conversations for a user
func (r *PostgresRepository) GetConversations(userID string) ([]map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		SELECT id, title, created_at, updated_at
		FROM conversations
		WHERE user_id = $1
		ORDER BY updated_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conversations []map[string]interface{}
	for rows.Next() {
		var id, title string
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &title, &createdAt, &updatedAt); err != nil {
			return nil, err
		}

		conversations = append(conversations, map[string]interface{}{
			"id":         id,
			"title":      title,
			"created_at": createdAt,
			"updated_at": updatedAt,
		})
	}

	return conversations, nil
}

// GetMessages returns all messages for a conversation
func (r *PostgresRepository) GetMessages(conversationID string) ([]map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		SELECT id, role, content, content_type, confidence, timestamp
		FROM messages
		WHERE conversation_id = $1
		ORDER BY timestamp ASC
	`

	rows, err := r.db.QueryContext(ctx, query, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []map[string]interface{}
	for rows.Next() {
		var id, role, content, contentType string
		var confidence float32
		var timestamp time.Time

		if err := rows.Scan(&id, &role, &content, &contentType, &confidence, &timestamp); err != nil {
			return nil, err
		}

		messages = append(messages, map[string]interface{}{
			"id":           id,
			"role":         role,
			"content":      content,
			"content_type": contentType,
			"confidence":   confidence,
			"timestamp":    timestamp,
		})
	}

	return messages, nil
}

// Close closes the database connection
func (r *PostgresRepository) Close() error {
	return r.db.Close()
}
