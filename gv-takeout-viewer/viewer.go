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

const indexTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Google Voice Takeout Viewer</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            line-height: 1.6;
            margin: 0;
            padding: 20px;
            background-color: #f4f4f4;
        }
        .container {
            max-width: 800px;
            margin: 0 auto;
            background-color: #fff;
            padding: 20px;
            border-radius: 5px;
            box-shadow: 0 0 10px rgba(0,0,0,0.1);
        }
        h1 {
            color: #333;
        }
        .conversation-list {
            list-style-type: none;
            padding: 0;
        }
        .conversation-item {
            background-color: #f9f9f9;
            border: 1px solid #ddd;
            margin-bottom: 10px;
            padding: 10px;
            border-radius: 3px;
        }
        .conversation-type {
            font-weight: bold;
            color: #555;
        }
        .conversation-timestamp {
            color: #888;
            font-size: 0.9em;
        }
        .pagination {
            margin-top: 20px;
            text-align: center;
        }
        .pagination a {
            display: inline-block;
            padding: 8px 16px;
            text-decoration: none;
            color: #333;
            background-color: #f1f1f1;
            border: 1px solid #ddd;
            margin: 0 4px;
        }
        .pagination a:hover {
            background-color: #ddd;
        }
        .pagination .active {
            background-color: #4CAF50;
            color: white;
            border: 1px solid #4CAF50;
        }
        .search-form {
            margin-bottom: 20px;
        }
        .search-form input[type="text"] {
            padding: 8px;
            width: 70%;
        }
        .search-form input[type="submit"] {
            padding: 8px 16px;
            background-color: #4CAF50;
            color: white;
            border: none;
            cursor: pointer;
        }
        .participants {
            font-style: italic;
            color: #666;
            margin-top: 5px;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>Google Voice Takeout Viewer</h1>
        <p>Welcome to the Google Voice Takeout Viewer. This application allows you to browse and search your Google Voice conversations.</p>
        <form class="search-form" action="/" method="GET">
            <input type="text" name="search" placeholder="Search conversations..." value="{{.SearchTerm}}">
            <input type="submit" value="Search">
        </form>
        <h2>Conversations</h2>
        <ul class="conversation-list">
            {{range .Conversations}}
            <li class="conversation-item">
                <span class="conversation-type">{{.Type}}</span>
                <span class="conversation-timestamp">{{.Timestamp.Format "Jan 02, 2006 15:04:05"}}</span>
                <div class="participants">
                    Participants:
                    {{range $index, $participant := .Participants}}
                        {{if $index}}, {{end}}
                        {{if $participant.Name}}{{$participant.Name}}{{else}}{{$participant.PhoneNumber}}{{end}}
                    {{end}}
                </div>
                <p>{{if .Transcript}}{{.Transcript}}{{else}}No transcript available{{end}}</p>
                <a href="/conversation/{{.ID}}">View Details</a>
            </li>
            {{else}}
            <li>No conversations found.</li>
            {{end}}
        </ul>
        <div class="pagination">
            {{if gt .CurrentPage 1}}
                <a href="/?page={{.PrevPage}}&search={{.SearchTerm}}">&laquo; Previous</a>
            {{end}}
            {{range .Pages}}
                {{if eq . $.CurrentPage}}
                    <a href="/?page={{.}}&search={{$.SearchTerm}}" class="active">{{.}}</a>
                {{else}}
                    <a href="/?page={{.}}&search={{$.SearchTerm}}">{{.}}</a>
                {{end}}
            {{end}}
            {{if lt .CurrentPage .TotalPages}}
                <a href="/?page={{.NextPage}}&search={{.SearchTerm}}">Next &raquo;</a>
            {{end}}
        </div>
    </div>
</body>
</html>
`

const conversationDetailTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Conversation Detail - Google Voice Takeout Viewer</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            line-height: 1.6;
            margin: 0;
            padding: 20px;
            background-color: #f4f4f4;
        }
        .container {
            max-width: 800px;
            margin: 0 auto;
            background-color: #fff;
            padding: 20px;
            border-radius: 5px;
            box-shadow: 0 0 10px rgba(0,0,0,0.1);
        }
        h1, h2 {
            color: #333;
        }
        .message-list {
            list-style-type: none;
            padding: 0;
        }
        .message-item {
            background-color: #f9f9f9;
            border: 1px solid #ddd;
            margin-bottom: 10px;
            padding: 10px;
            border-radius: 3px;
        }
        .message-sender {
            font-weight: bold;
            color: #555;
        }
        .message-timestamp {
            color: #888;
            font-size: 0.9em;
        }
        .back-link {
            display: inline-block;
            margin-top: 20px;
            padding: 8px 16px;
            background-color: #4CAF50;
            color: white;
            text-decoration: none;
            border-radius: 3px;
        }
        .message-image {
            max-width: 100%;
            height: auto;
            margin-top: 10px;
        }
        .participants {
            font-style: italic;
            color: #666;
            margin-bottom: 15px;
        }
        .transcript {
            background-color: #f0f0f0;
            padding: 10px;
            border-radius: 3px;
            margin-bottom: 15px;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>Conversation Detail</h1>
        <h2>{{.Conversation.Type}} - {{.Conversation.Timestamp.Format "Jan 02, 2006 15:04:05"}}</h2>
        <div class="participants">
            Participants:
            {{range $index, $participant := .Conversation.Participants}}
                {{if $index}}, {{end}}
                {{if $participant.Name}}{{$participant.Name}}{{else}}{{$participant.PhoneNumber}}{{end}}
            {{end}}
        </div>
        <div class="transcript">
            <h3>Transcript:</h3>
            <pre>{{.Conversation.Transcript}}</pre>
        </div>
        <h3>Messages:</h3>
        <ul class="message-list">
            {{range .Messages}}
            <li class="message-item">
                <span class="message-sender">{{.Sender}}</span>
                <span class="message-timestamp">{{.Timestamp.Format "Jan 02, 2006 15:04:05"}}</span>
                <p>{{.Content}}</p>
                {{if .ImageURL}}
                <img src="{{.ImageURL}}" alt="Message Image" class="message-image">
                {{end}}
            </li>
            {{else}}
            <li>No messages found for this conversation.</li>
            {{end}}
        </ul>
        <a href="/" class="back-link">Back to Conversations</a>
    </div>
</body>
</html>
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
	Name        string
	PhoneNumber string
}

type Message struct {
	ID        int
	Timestamp time.Time
	Sender    string
	Content   string
	ImageURL  *string
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

	templates = template.Must(template.New("index").Parse(indexTemplate))
	template.Must(templates.New("conversation").Parse(conversationDetailTemplate))

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

	if err := templates.ExecuteTemplate(w, "index", data); err != nil {
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

	if err := templates.ExecuteTemplate(w, "conversation", data); err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %v", err), http.StatusInternalServerError)
	}
}

func getConversations(limit, offset int, searchTerm string) ([]Conversation, error) {
	query := `
		SELECT c.id, c.type, c.timestamp, c.duration
		FROM conversations c
		WHERE c.transcript LIKE ? OR c.id IN (
			SELECT DISTINCT conversation_id
			FROM messages
			WHERE content LIKE ?
		)
		ORDER BY c.timestamp DESC
		LIMIT ? OFFSET ?
	`
	searchPattern := "%" + searchTerm + "%"
	rows, err := db.Query(query, searchPattern, searchPattern, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query conversations: %v", err)
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
		SELECT name, phone_number
		FROM participants
		WHERE conversation_id = ?
	`
	rows, err := db.Query(query, conversationID)
	if err != nil {
		return nil, fmt.Errorf("failed to query participants: %v", err)
	}
	defer rows.Close()

	var participants []Participant
	for rows.Next() {
		var p Participant
		err := rows.Scan(&p.Name, &p.PhoneNumber)
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
	query := "SELECT COUNT(*) FROM conversations WHERE transcript LIKE ?"
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
		FROM conversations
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
		SELECT m.id, m.timestamp, m.sender, m.content, i.image_url
		FROM messages m
		LEFT JOIN images i ON m.id = i.message_id
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
		err := rows.Scan(&m.ID, &m.Timestamp, &m.Sender, &m.Content, &m.ImageURL)
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

// Schema
// conversation.type can be missed_call received_call voicemail chat placed_call
// `CREATE TABLE IF NOT EXISTS conversations (
// 	id INTEGER PRIMARY KEY AUTOINCREMENT,
// 	type TEXT,
// 	timestamp DATETIME,
// 	duration TEXT,
// 	transcript TEXT,
// 	source_file TEXT
// )`,
// `CREATE TABLE IF NOT EXISTS participants (
// 	id INTEGER PRIMARY KEY AUTOINCREMENT,
// 	conversation_id INTEGER,
// 	name TEXT,
// 	phone_number TEXT,
// 	FOREIGN KEY (conversation_id) REFERENCES conversations (id)
// )`,
// `CREATE TABLE IF NOT EXISTS messages (
// 	id INTEGER PRIMARY KEY AUTOINCREMENT,
// 	conversation_id INTEGER,
// 	timestamp DATETIME,
// 	sender TEXT,
// 	sender_number TEXT,
// 	content TEXT,
// 	FOREIGN KEY (conversation_id) REFERENCES conversations (id)
// )`,
// `CREATE TABLE IF NOT EXISTS images (
// 	id INTEGER PRIMARY KEY AUTOINCREMENT,
// 	message_id INTEGER,
// 	image_url TEXT,
// 	FOREIGN KEY (message_id) REFERENCES messages (id)
// )`,

func getTranscript(conversationID int, conversationType string) (string, error) {
	if conversationType == "voicemail" {
		// For voicemail, we already have the transcript in the conversations table
		var transcript string
		err := db.QueryRow("SELECT transcript FROM conversations WHERE id = ?", conversationID).Scan(&transcript)
		if err != nil {
			return "", fmt.Errorf("failed to fetch voicemail transcript: %v", err)
		}
		return transcript, nil
	}

	// For chat messages, build the transcript from the messages table
	query := `
		SELECT sender, content
		FROM messages
		WHERE conversation_id = ?
		ORDER BY timestamp ASC
		LIMIT 5
	`
	rows, err := db.Query(query, conversationID)
	if err != nil {
		return "", fmt.Errorf("failed to query messages for transcript: %v", err)
	}
	defer rows.Close()

	var transcript strings.Builder
	for rows.Next() {
		var sender, content string
		err := rows.Scan(&sender, &content)
		if err != nil {
			return "", fmt.Errorf("failed to scan message row: %v", err)
		}
		transcript.WriteString(fmt.Sprintf("%s: %s\n", sender, content))
	}

	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("error iterating message rows: %v", err)
	}

	return transcript.String(), nil
}
