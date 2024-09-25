package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/html"
	_ "modernc.org/sqlite"
)

type Conversation struct {
	Type         string            `json:"type"`
	Participants map[string]string `json:"participants"`
	Timestamp    time.Time         `json:"timestamp"`
	Duration     string            `json:"duration,omitempty"`
	Messages     []Message         `json:"messages,omitempty"`
	Transcript   string            `json:"transcript,omitempty"`
	SourceFile   string            `json:"source_file"`
}

type Message struct {
	Timestamp    time.Time `json:"timestamp"`
	Sender       string    `json:"sender"`
	SenderNumber string    `json:"sender_number"`
	Content      string    `json:"content"`
	Images       []string  `json:"images,omitempty"`
}

var (
	format = flag.String("format", "json", "Output format: json or sqlite")
)

func main() {
	flag.Parse()

	if *format != "json" && *format != "sqlite" {
		log.Fatal("Invalid format. Use 'json' or 'sqlite'")
	}

	files, err := filepath.Glob("*.html")
	if err != nil {
		log.Fatal(err)
	}

	parentLgr := slog.Default()

	var output func(Conversation)
	switch *format {
	case "json":
		output = outputJSON
	case "sqlite":
		db := initSQLiteDB()
		defer db.Close()
		output = func(conv Conversation) {
			outputSQLite(db, conv)
		}
	}

	for _, file := range files {
		lgr := parentLgr.With("file", file)
		f, err := os.Open(file)
		if err != nil {
			lgr.Error("error opening file", "err", err)
			continue
		}

		conversation, err := parseFile(lgr, f, file)
		if err != nil {
			lgr.Error("error parsing file", "err", err)
			f.Close()
			continue
		}

		f.Close()

		if conversation.Type == "" {
			lgr.Error("failed to parse file correctly")
			continue
		}

		conversation.SourceFile = file
		output(conversation)
	}
}

func outputJSON(conversation Conversation) {
	jsonData, err := json.Marshal(conversation)
	if err != nil {
		log.Printf("error marshaling JSON for file %s: %v", conversation.SourceFile, err)
		return
	}
	fmt.Println(string(jsonData))
}

func initSQLiteDB() *sql.DB {
	dbName := "conversations.db"
	db, err := sql.Open("sqlite", dbName)
	if err != nil {
		log.Fatalf("Failed to open SQLite database: %v", err)
	}

	_, err = db.Exec("PRAGMA journal_mode=WAL")
	if err != nil {
		log.Fatalf("PRAGMA journal_mode=WAL error: %s", err)
	}

	createTables(db)
	log.Printf("SQLite database initialized: %s", dbName)
	return db
}

func outputSQLite(db *sql.DB, conv Conversation) {
	insertConversation(db, conv)
}

func createTables(db *sql.DB) {
	createTableQueries := []string{
		`CREATE TABLE IF NOT EXISTS contact (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT,
			phone_number TEXT,
			UNIQUE(name, phone_number)
		)`,
		`CREATE TABLE IF NOT EXISTS conversation (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT,
			timestamp DATETIME,
			duration TEXT,
			transcript TEXT,
			source_file TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS participant (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			conversation_id INTEGER,
			contact_id INTEGER,
			FOREIGN KEY (conversation_id) REFERENCES conversation (id),
			FOREIGN KEY (contact_id) REFERENCES contact (id)
		)`,
		`CREATE TABLE IF NOT EXISTS message (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			conversation_id INTEGER,
			timestamp DATETIME,
			sender_contact_id INTEGER,
			content TEXT,
			FOREIGN KEY (conversation_id) REFERENCES conversation (id),
			FOREIGN KEY (sender_contact_id) REFERENCES contact (id)
		)`,
		`CREATE TABLE IF NOT EXISTS image (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			message_id INTEGER,
			image_url TEXT,
			FOREIGN KEY (message_id) REFERENCES message (id)
		)`,
		`CREATE TABLE IF NOT EXISTS media_file (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			image_id INTEGER,
			file_name TEXT,
			content BLOB,
			FOREIGN KEY (image_id) REFERENCES image (id)
		)`,
	}

	for _, query := range createTableQueries {
		if _, err := db.Exec(query); err != nil {
			log.Fatalf("Failed to create table: %v", err)
		}
	}
}

func insertConversation(db *sql.DB, conv Conversation) {
	tx, err := db.Begin()
	if err != nil {
		log.Printf("Failed to begin transaction: %v", err)
		return
	}
	defer tx.Rollback()

	// Insert conversation
	convStmt, err := tx.Prepare("INSERT INTO conversation (type, timestamp, duration, transcript, source_file) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		log.Printf("Failed to prepare conversation statement: %v", err)
		return
	}
	defer convStmt.Close()

	result, err := convStmt.Exec(conv.Type, conv.Timestamp, conv.Duration, conv.Transcript, conv.SourceFile)
	if err != nil {
		log.Printf("Failed to insert conversation: %v", err)
		return
	}

	convID, err := result.LastInsertId()
	if err != nil {
		log.Printf("Failed to get last insert ID: %v", err)
		return
	}

	// Insert contacts and participants
	contactStmt, err := tx.Prepare("INSERT OR IGNORE INTO contact (name, phone_number) VALUES (?, ?)")
	if err != nil {
		log.Printf("Failed to prepare contact statement: %v", err)
		return
	}
	defer contactStmt.Close()

	partStmt, err := tx.Prepare("INSERT INTO participant (conversation_id, contact_id) VALUES (?, ?)")
	if err != nil {
		log.Printf("Failed to prepare participant statement: %v", err)
		return
	}
	defer partStmt.Close()

	contactIDs := make(map[string]int64)

	for name, number := range conv.Participants {
		_, err := contactStmt.Exec(name, number)
		if err != nil {
			log.Printf("Failed to insert contact: %v", err)
			return
		}

		var contactID int64
		err = tx.QueryRow("SELECT id FROM contact WHERE name = ? AND phone_number = ?", name, number).Scan(&contactID)
		if err != nil {
			log.Printf("Failed to get contact ID: %v", err)
			return
		}

		contactIDs[name] = contactID

		_, err = partStmt.Exec(convID, contactID)
		if err != nil {
			log.Printf("Failed to insert participant: %v", err)
			return
		}
	}

	// Insert messages and images
	msgStmt, err := tx.Prepare("INSERT INTO message (conversation_id, timestamp, sender_contact_id, content) VALUES (?, ?, ?, ?)")
	if err != nil {
		log.Printf("Failed to prepare message statement: %v", err)
		return
	}
	defer msgStmt.Close()

	imgStmt, err := tx.Prepare("INSERT INTO image (message_id, image_url) VALUES (?, ?)")
	if err != nil {
		log.Printf("Failed to prepare image statement: %v", err)
		return
	}
	defer imgStmt.Close()

	mediaStmt, err := tx.Prepare("INSERT INTO media_file (image_id, file_name, content) VALUES (?, ?, ?)")
	if err != nil {
		log.Printf("Failed to prepare media file statement: %v", err)
		return
	}
	defer mediaStmt.Close()

	for _, msg := range conv.Messages {
		senderContactID, ok := contactIDs[msg.Sender]
		if !ok {
			log.Printf("Failed to find contact ID for sender: %s", msg.Sender)
			return
		}

		msgResult, err := msgStmt.Exec(convID, msg.Timestamp, senderContactID, msg.Content)
		if err != nil {
			log.Printf("Failed to insert message: %v", err)
			return
		}

		msgID, err := msgResult.LastInsertId()
		if err != nil {
			log.Printf("Failed to get last insert ID for message: %v", err)
			return
		}

		for _, img := range msg.Images {
			imgResult, err := imgStmt.Exec(msgID, img)
			if err != nil {
				log.Printf("Failed to insert image: %v", err)
				return
			}

			imgID, err := imgResult.LastInsertId()
			if err != nil {
				log.Printf("Failed to get last insert ID for image: %v", err)
				return
			}

			if err := insertMediaFile(tx, mediaStmt, imgID, img); err != nil {
				log.Printf("Failed to insert media file: %v", err)
				return
			}
		}
	}

	if err := tx.Commit(); err != nil {
		log.Printf("Failed to commit transaction: %v", err)
	}
}

func insertMediaFile(tx *sql.Tx, stmt *sql.Stmt, imgID int64, imageURL string) error {
	fullPath, err := findMediaFile(imageURL)
	if err != nil {
		return fmt.Errorf("failed to find media file for %s: %s", imageURL, err)
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		log.Printf("Failed to read img file %s", fullPath)
		return fmt.Errorf("failed to read media file: %v", err)
	}

	_, err = stmt.Exec(imgID, fullPath, content)
	if err != nil {
		return fmt.Errorf("failed to insert media file: %v", err)
	}

	return nil
}

func parseFile(lgr *slog.Logger, r io.Reader, filename string) (Conversation, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return Conversation{}, err
	}

	conversation := Conversation{
		Participants: make(map[string]string),
	}
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "title":
				title := extractTitle(n)
				title = strings.ReplaceAll(title, "\n", " ")
				parts := strings.Split(title, " to ")

				// Add participants from the title if they're not already in the map
				if len(parts) == 2 {
					sender := strings.TrimSpace(parts[0])
					recipient := strings.TrimSpace(parts[1])
					if _, exists := conversation.Participants[sender]; !exists {
						conversation.Participants[sender] = ""
					}
					if _, exists := conversation.Participants[recipient]; !exists {
						conversation.Participants[recipient] = ""
					}
				}

			case "div":
				for _, a := range n.Attr {
					if a.Key == "class" {
						switch a.Val {
						case "hChatLog hfeed":
							conversation.Type = "chat"
							for k, v := range parseParticipants(lgr, n) {
								conversation.Participants[k] = v
							}
							conversation.Messages = parseMessages(lgr, n)
							if len(conversation.Messages) > 0 {
								conversation.Timestamp = conversation.Messages[0].Timestamp
							}
						case "haudio":
							conversation = parseCallOrVoicemail(lgr, n)
						}
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)

	return conversation, nil
}

func parseCallOrVoicemail(lgr *slog.Logger, n *html.Node) Conversation {
	var conv Conversation
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "div":
				for _, a := range n.Attr {
					if a.Key == "class" && a.Val == "contributor vcard" {
						conv.Participants = parseParticipants(lgr, n)
					}
				}
			case "span":
				for _, a := range n.Attr {
					if a.Key == "class" && a.Val == "full-text" {
						conv.Transcript = extractText(n)
					}
				}
			case "abbr":
				for _, a := range n.Attr {
					if a.Key == "class" {
						switch a.Val {
						case "published":
							if t, err := parseTimestamp(n); err == nil {
								conv.Timestamp = t
							} else {
								lgr.Error("parse time err", "err", err)
							}
						case "duration":
							conv.Duration = strings.Trim(extractText(n), "()")
						}
					}
				}
			}
		} else if n.Type == html.TextNode {
			text := strings.TrimSpace(n.Data)
			if strings.Contains(text, "Voicemail") {
				conv.Type = "voicemail"
			} else if strings.Contains(text, "Placed call") {
				conv.Type = "placed_call"
			} else if strings.Contains(text, "Received call") {
				conv.Type = "received_call"
			} else if strings.Contains(text, "Missed call") {
				conv.Type = "missed_call"
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	return conv
}

func parseTimestamp(n *html.Node) (time.Time, error) {
	for _, a := range n.Attr {
		if a.Key == "title" {
			return time.Parse(time.RFC3339, a.Val)
		}
	}
	return time.Time{}, fmt.Errorf("no title attribute found for timestamp")
}

func parseParticipants(lgr *slog.Logger, n *html.Node) map[string]string {
	participants := make(map[string]string)

	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, a := range n.Attr {
				if a.Key == "class" && a.Val == "fn" {
					name := extractText(n)
					number := extractPhoneNumber(n.Parent)
					participants[name] = number
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)

	return participants
}

func extractPhoneNumber(n *html.Node) string {
	for _, a := range n.Attr {
		if a.Key == "href" && strings.HasPrefix(a.Val, "tel:") {
			return strings.TrimPrefix(a.Val, "tel:")
		}
	}
	return ""
}

func extractTitle(n *html.Node) string {
	var title string
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "title" {
			title = extractText(n)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	return title
}

func parseMessages(lgr *slog.Logger, n *html.Node) []Message {
	var messages []Message
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" {
			for _, a := range n.Attr {
				if a.Key == "class" && a.Val == "message" {
					msg := parseMessage(n)
					messages = append(messages, msg)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	return messages
}

func parseMessage(n *html.Node) Message {
	var msg Message
	var senderName, senderNumber string
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "abbr":
				for _, a := range n.Attr {
					if a.Key == "class" && a.Val == "dt" {
						msg.Timestamp = parseMessageTimestamp(n)
					}
				}
			case "cite":
				senderName, senderNumber = parseSenderAndNumber(n)
			case "q":
				msg.Content = extractText(n)
			case "img":
				for _, a := range n.Attr {
					if a.Key == "src" {
						msg.Images = append(msg.Images, a.Val)
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	msg.Sender = senderName
	msg.SenderNumber = senderNumber
	return msg
}

func parseSenderAndNumber(n *html.Node) (string, string) {
	var sender, number string
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "abbr", "span":
				for _, a := range n.Attr {
					if a.Key == "class" && a.Val == "fn" {
						sender = extractText(n)
					}
				}
			case "a":
				for _, a := range n.Attr {
					if a.Key == "href" && strings.HasPrefix(a.Val, "tel:") {
						number = strings.TrimPrefix(a.Val, "tel:")
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	return sender, number
}

func parseMessageTimestamp(n *html.Node) time.Time {
	for _, a := range n.Attr {
		if a.Key == "title" {
			if t, err := time.Parse(time.RFC3339, a.Val); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}

func parseSender(n *html.Node) string {
	var sender string
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "abbr" {
			for _, a := range n.Attr {
				if a.Key == "class" && a.Val == "fn" {
					sender = extractText(n)
					return
				}
			}
		}
		if n.Type == html.ElementNode && n.Data == "span" {
			for _, a := range n.Attr {
				if a.Key == "class" && a.Val == "fn" {
					sender = extractText(n)
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	return sender
}

func extractText(n *html.Node) string {
	var text string
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.TextNode {
			text += n.Data
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	return strings.TrimSpace(text)
}

func findMediaFile(relativePath string) (string, error) {
	parts := strings.Split(relativePath, " ")
	last := parts[len(parts)-1]

	look := func(glob string) (string, error) {
		matches, err := filepath.Glob("*" + glob + "*")
		if err != nil {
			return "", err
		}
		for _, match := range matches {
			if strings.HasSuffix(match, ".html") {
				continue
			}
			return match, nil
		}
		return "", nil
	}

	match, err := look(last)
	if err != nil {
		return "", err
	}
	if match != "" {
		return match, nil
	}

	lastParts := strings.Split(last, "-")
	if len(lastParts) > 2 {
		lastParts = lastParts[:len(lastParts)-1]
		last = strings.Join(lastParts, "-")

		match, err := look(last)
		if err != nil {
			return "", err
		}
		if match != "" {
			return match, nil
		}
	}

	return "", fmt.Errorf("no matching media file found for %s", relativePath)
}
