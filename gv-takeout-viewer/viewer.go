package main

import (
	"database/sql"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var dbPath = flag.String("db", "conversations.db", "Path to sqlite db")
var addr = flag.String("addr", ":8080", "HTTP server address")

var db *sql.DB
var templates *template.Template

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

	http.HandleFunc("GET /", indexHandler)
	http.HandleFunc("GET /group/{key}", groupHandler)

	log.Printf("Starting server on %s", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	groups, err := getGroups()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch groups: %v", err), http.StatusInternalServerError)
		return
	}

	data := struct {
		Groups []Group
	}{
		Groups: groups,
	}

	if err := templates.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %v", err), http.StatusInternalServerError)
	}
}

func groupHandler(w http.ResponseWriter, r *http.Request) {
	groupKey := r.PathValue("key")

	contactIDStrs := strings.Split(groupKey, ",")

	contactIDs := make([]int, len(contactIDStrs))
	for i, cid := range contactIDStrs {
		id, err := strconv.Atoi(cid)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to parse group id: %s", err), http.StatusBadRequest)
			return
		}
		contactIDs[i] = id
	}

	msgs, err := getMessagesForGroup(contactIDs)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch messages: %s", err), http.StatusInternalServerError)
		return
	}

	g := Group{
		Key: groupKey,
	}

	seenParticipants := make(map[int]struct{})
	for _, msg := range msgs {

		if msg.Timestamp.After(g.Timestamp) {
			g.Timestamp = msg.Timestamp
		}

		if _, seen := seenParticipants[msg.SenderContactID]; seen {
			continue
		}
		p := Participant{
			ContactID:   msg.SenderContactID,
			Name:        msg.SenderName,
			PhoneNumber: msg.SenderNumber,
		}
		g.Participants = append(g.Participants, p)
		seenParticipants[p.ContactID] = struct{}{}
	}

	data := struct {
		Group    Group
		Messages []Message
	}{
		Group:    g,
		Messages: msgs,
	}

	if err := templates.ExecuteTemplate(w, "group.html", data); err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %s", err), http.StatusInternalServerError)
	}
}

func getMessagesForGroup(contactIDs []int) ([]Message, error) {
	contactMap := make(map[int]struct{})
	for _, contactID := range contactIDs {
		contactMap[contactID] = struct{}{}
	}

	query := "SELECT contact_id, conversation_id from participant order by conversation_id"
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query participant: %s", err)
	}

	var (
		conversationIDs []int

		seenContactsForConversation = make(map[int]struct{})
		currentConvID               = -1
		validConv                   = true
	)

	for rows.Next() {
		var contactID, conversationID int
		var err = rows.Scan(&contactID, &conversationID)
		if err != nil {
			return nil, fmt.Errorf("failed to scan conversation row: %s", err)
		}

		if currentConvID < 0 {
			currentConvID = conversationID
		} else if currentConvID != conversationID {
			if validConv && len(seenContactsForConversation) == len(contactIDs) {
				conversationIDs = append(conversationIDs, currentConvID)
			}

			validConv = true
			currentConvID = conversationID
			seenContactsForConversation = make(map[int]struct{})
		}

		if _, found := contactMap[contactID]; !found {
			validConv = false
		} else {
			seenContactsForConversation[contactID] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating conversation rows: %v", err)
	}

	if validConv {
		conversationIDs = append(conversationIDs, currentConvID)
	}

	qs := make([]string, len(conversationIDs))
	for i := range qs {
		qs[i] = "?"
	}
	qsStr := strings.Join(qs, ",")

	query = `
		SELECT m.id, m.timestamp, m.sender_contact_id, c.name, c.phone_number, m.content, i.image_url
		FROM message m
		LEFT JOIN image i ON m.id = i.message_id
		LEFT JOIN contact c ON m.sender_contact_id = c.id
		WHERE m.conversation_id in (%s)
		ORDER BY m.timestamp DESC
	`
	query = fmt.Sprintf(query, qsStr)
	convIDs := make([]any, len(conversationIDs))
	for i, cid := range conversationIDs {
		convIDs[i] = cid
	}
	rows, err = db.Query(query, convIDs...)
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

type Group struct {
	Key                string
	Type               string
	Timestamp          time.Time
	LastConversationID int
	Participants       []Participant
	RecentMessages     []Message
}

func getGroups() ([]Group, error) {
	query := `SELECT conversation.id, conversation.type, conversation.timestamp,
            contact.id, contact.name, contact.phone_number
            FROM conversation, participant, contact
            WHERE participant.conversation_id = conversation.id
            AND participant.contact_id = contact.id
            ORDER BY conversation.timestamp DESC`
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query groups: %v", err)
	}

	var (
		groupsByParticipants = make(map[string]Group)
		groups               = make([]Group, 0, 1000)
		currentConvID        = -1

		currentConversation Conversation
		currentParticipants []Participant
	)

	makeGroup := func() error {
		contactIDs := make([]int, len(currentParticipants))
		for i, p := range currentParticipants {
			contactIDs[i] = p.ContactID
		}
		sort.Sort(sort.IntSlice(contactIDs))

		contactIDStrs := make([]string, len(contactIDs))
		for i, cid := range contactIDs {
			contactIDStrs[i] = strconv.Itoa(cid)
		}

		groupKey := strings.Join(contactIDStrs, ",")

		if _, found := groupsByParticipants[groupKey]; !found {
			msgs, err := getMessagesByConversationID(currentConversation.ID)
			if err != nil {
				return fmt.Errorf("get messages for conversation %d err: %s", currentConversation.ID, err)
			}
			group := Group{
				Key:                groupKey,
				Type:               currentConversation.Type,
				Timestamp:          currentConversation.Timestamp,
				LastConversationID: currentConversation.ID,
				Participants:       currentParticipants,
				RecentMessages:     msgs,
			}
			groupsByParticipants[groupKey] = group
			groups = append(groups, group)
		}

		return nil
	}

	for rows.Next() {
		var c Conversation
		var p Participant
		var err = rows.Scan(&c.ID, &c.Type, &c.Timestamp, &p.ContactID, &p.Name, &p.PhoneNumber)
		if err != nil {
			return nil, fmt.Errorf("failed to scan conversation+participant+contact row: %v", err)
		}

		if currentConvID < 0 {
			currentConvID = c.ID
		} else if currentConvID != c.ID {
			err = makeGroup()
			if err != nil {
				return nil, err
			}
			currentParticipants = make([]Participant, 0)
			currentConvID = c.ID
			currentConversation = c
		}

		currentParticipants = append(currentParticipants, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating conversation rows: %v", err)
	}

	err = makeGroup()

	return groups, nil
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
