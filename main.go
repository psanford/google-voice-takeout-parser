package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/html"
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

func main() {
	files, err := filepath.Glob("*.html")
	if err != nil {
		log.Fatal(err)
	}

	parentLgr := slog.Default()

	for _, file := range files {
		lgr := parentLgr.With("file", file)
		f, err := os.Open(file)
		if err != nil {
			lgr.Error("error opening file", "err", err)
			continue
		}
		defer f.Close()

		conversation, err := parseFile(lgr, f)
		if err != nil {
			lgr.Error("error parsing file", "err", err)
			continue
		}

		if conversation.Type == "" {
			lgr.Error("falsed to parse file correctly")
			os.Exit(1)
		}

		conversation.SourceFile = file

		jsonData, err := json.Marshal(conversation)
		if err != nil {
			lgr.Error("error marshaling JSON file", "err", err)
			continue
		}

		fmt.Println(string(jsonData))
	}
}

func parseFile(lgr *slog.Logger, r io.Reader) (Conversation, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return Conversation{}, err
	}

	var conversation Conversation
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "div":
				for _, a := range n.Attr {
					if a.Key == "class" {
						switch a.Val {
						case "hChatLog hfeed":
							conversation.Type = "chat"
							conversation.Participants = parseParticipants(lgr, n)
							conversation.Messages = parseMessages(lgr, n)
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
				msg.Sender, msg.SenderNumber = parseSenderAndNumber(n)
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
