package main

import (
	"database/sql"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var dbPath = flag.String("db", "conversations.db", "Path to sqlite db")
var addr = flag.String("addr", ":8080", "HTTP server address")

var db *sql.DB
var templates *template.Template

const conversationDetailTemplate = `
`

type Conversation struct {
	ID           int
	Type         string
	Timestamp    time.Time
	Duration     string
	Transcript   string
	Participants []Participant
}

type Participant struct {
	ID          int
	ContactID   int
	Name        string
	PhoneNumber string
}

type Message struct {
	ID              int
	Timestamp       time.Time
	SenderContactID int
	SenderName      string
	SenderNumber    string
	Content         string
	ImageURL        *string
}

func main() {
	flag.Parse()

	var err error
	db, err = sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Fatalf("Failed to open SQLite database: %v", err)
	}
	defer db.Close()

	_, err = db.Exec("PRAGMA journal_mode=WAL")
	if err != nil {
		log.Fatalf("PRAGMA journal_mode=WAL error: %s", err)
	}

	templates = template.New("")
	_, err = templates.ParseGlob("templates/*.html")
	if err != nil {
		log.Fatalf("parse templates err: %s", err)
	}

	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/conversation/", conversationDetailHandler)

	log.Printf("Starting server on %s", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	pageStr := r.URL.Query().Get("page")
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	searchTerm := r.URL.Query().Get("search")

	limit := 10
	offset := (page - 1) * limit

	conversations, err := getConversations(limit, offset, searchTerm)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch conversations: %v", err), http.StatusInternalServerError)
		return
	}

	totalCount, err := getTotalConversationCount(searchTerm)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch total conversation count: %v", err), http.StatusInternalServerError)
		return
	}

	totalPages := (totalCount + limit - 1) / limit

	var pages []int
	for i := 1; i <= totalPages; i++ {
		pages = append(pages, i)
	}

	data := struct {
		Conversations []Conversation
		CurrentPage   int
		TotalPages    int
		PrevPage      int
		NextPage      int
		Pages         []int
		SearchTerm    string
	}{
		Conversations: conversations,
		CurrentPage:   page,
		TotalPages:    totalPages,
		PrevPage:      page - 1,
		NextPage:      page + 1,
		Pages:         pages,
		SearchTerm:    searchTerm,
	}

	if err := templates.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %v", err), http.StatusInternalServerError)
	}
}

func conversationDetailHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Path[len("/conversation/"):]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid conversation ID", http.StatusBadRequest)
		return
	}

	conversation, err := getConversationByID(id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch conversation: %v", err), http.StatusInternalServerError)
		return
	}

	transcript, err := getTranscript(id, conversation.Type)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch transcript: %v", err), http.StatusInternalServerError)
		return
	}
	conversation.Transcript = transcript

	messages, err := getMessagesByConversationID(id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch messages: %v", err), http.StatusInternalServerError)
		return
	}

	participants, err := getParticipants(id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch participants: %v", err), http.StatusInternalServerError)
		return
	}
	conversation.Participants = participants

	data := struct {
		Conversation Conversation
		Messages     []Message
	}{
		Conversation: conversation,
		Messages:     messages,
	}

	if err := templates.ExecuteTemplate(w, "conversation.html", data); err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %v", err), http.StatusInternalServerError)
	}
}

func getConversations(limit, offset int, searchTerm string) ([]Conversation, error) {
	query := `
		SELECT DISTINCT c.id, c.type, c.timestamp, c.duration
		FROM conversation c
		LEFT JOIN message m ON c.id = m.conversation_id
		LEFT JOIN contact ct ON m.sender_contact_id = ct.id
		WHERE c.transcript LIKE ? OR m.content LIKE ? OR ct.name LIKE ?
		ORDER BY c.timestamp DESC
		LIMIT ? OFFSET ?
	`
	searchPattern := "%" + searchTerm + "%"
	rows, err := db.Query(query, searchPattern, searchPattern, searchPattern, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query conversation: %v", err)
	}
	defer rows.Close()

	var conversations []Conversation
	for rows.Next() {
		var c Conversation
		err := rows.Scan(&c.ID, &c.Type, &c.Timestamp, &c.Duration)
		if err != nil {
			return nil, fmt.Errorf("failed to scan conversation row: %v", err)
		}

		// Fetch participants for this conversation
		participants, err := getParticipants(c.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get participants for conversation %d: %v", c.ID, err)
		}
		c.Participants = participants

		// Get transcript
		transcript, err := getTranscript(c.ID, c.Type)
		if err != nil {
			return nil, fmt.Errorf("failed to get transcript for conversation %d: %v", c.ID, err)
		}
		c.Transcript = transcript

		conversations = append(conversations, c)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating conversation rows: %v", err)
	}

	return conversations, nil
}

func getParticipants(conversationID int) ([]Participant, error) {
	query := `
		SELECT p.id, p.contact_id, c.name, c.phone_number
		FROM participant p
		JOIN contact c ON p.contact_id = c.id
		WHERE p.conversation_id = ?
	`
	rows, err := db.Query(query, conversationID)
	if err != nil {
		return nil, fmt.Errorf("failed to query participant: %v", err)
	}
	defer rows.Close()

	var participants []Participant
	for rows.Next() {
		var p Participant
		err := rows.Scan(&p.ID, &p.ContactID, &p.Name, &p.PhoneNumber)
		if err != nil {
			return nil, fmt.Errorf("failed to scan participant row: %v", err)
		}
		participants = append(participants, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating participant rows: %v", err)
	}

	return participants, nil
}

func getTotalConversationCount(searchTerm string) (int, error) {
	query := "SELECT COUNT(*) FROM conversation WHERE transcript LIKE ?"
	searchPattern := "%" + searchTerm + "%"
	var count int
	err := db.QueryRow(query, searchPattern).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get total conversation count: %v", err)
	}
	return count, nil
}

func getConversationByID(id int) (Conversation, error) {
	query := `
		SELECT id, type, timestamp, duration, transcript
		FROM conversation
		WHERE id = ?
	`
	var c Conversation
	err := db.QueryRow(query, id).Scan(&c.ID, &c.Type, &c.Timestamp, &c.Duration, &c.Transcript)
	if err != nil {
		return Conversation{}, fmt.Errorf("failed to fetch conversation: %v", err)
	}
	return c, nil
}

func getMessagesByConversationID(conversationID int) ([]Message, error) {
	query := `
		SELECT m.id, m.timestamp, m.sender_contact_id, c.name, c.phone_number, m.content, i.image_url
		FROM message m
		LEFT JOIN image i ON m.id = i.message_id
		LEFT JOIN contact c ON m.sender_contact_id = c.id
		WHERE m.conversation_id = ?
		ORDER BY m.timestamp ASC
	`
	rows, err := db.Query(query, conversationID)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %v", err)
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		err := rows.Scan(&m.ID, &m.Timestamp, &m.SenderContactID, &m.SenderName, &m.SenderNumber, &m.Content, &m.ImageURL)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message row: %v", err)
		}
		messages = append(messages, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating message rows: %v", err)
	}

	return messages, nil
}

func getTranscript(conversationID int, conversationType string) (string, error) {
	if conversationType == "voicemail" {
		// For voicemail, we already have the transcript in the conversation table
		var transcript string
		err := db.QueryRow("SELECT transcript FROM conversation WHERE id = ?", conversationID).Scan(&transcript)
		if err != nil {
			return "", fmt.Errorf("failed to fetch voicemail transcript: %v", err)
		}
		return transcript, nil
	}

	// For chat messages, build the transcript from the messages table
	query := `
		SELECT c.name, m.content
		FROM message m
		JOIN contact c ON m.sender_contact_id = c.id
		WHERE m.conversation_id = ?
		ORDER BY m.timestamp ASC
		LIMIT 5
	`
	rows, err := db.Query(query, conversationID)
	if err != nil {
		return "", fmt.Errorf("failed to query message for transcript: %v", err)
	}
	defer rows.Close()

	var transcript strings.Builder
	for rows.Next() {
		var senderName, content string
		err := rows.Scan(&senderName, &content)
		if err != nil {
			return "", fmt.Errorf("failed to scan message row: %v", err)
		}
		transcript.WriteString(fmt.Sprintf("%s: %s\n", senderName, content))
	}

	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("error iterating message rows: %v", err)
	}

	return transcript.String(), nil
}
