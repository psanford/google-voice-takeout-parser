package main

import (
	"log"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

func parseHTML(input string) (Conversation, error) {
	r := strings.NewReader(input)

	return parseFile(slog.Default(), r)
}

func TestParseVoicemail(t *testing.T) {
	input, err := os.ReadFile("testdata/voicemail.html")
	if err != nil {
		log.Fatal(err)
	}

	conv, err := parseHTML(string(input))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	expected := Conversation{
		Type: "voicemail",
		Participants: map[string]string{
			"Sleve Mcdichael": "+11111111111",
		},
		Timestamp:  time.Date(2018, 7, 23, 9, 23, 31, 0, time.FixedZone("Pacific Time", -7*60*60)),
		Duration:   "00:00:18",
		Transcript: "Hi Peter, this is Sleve Mcdichael. I'm the manager. I believe you have internet. I just have some quick questions for you. Thank you.",
	}

	if conv.Type != expected.Type {
		t.Errorf("Expected type %s, got %s", expected.Type, conv.Type)
	}
	if !conv.Timestamp.Equal(expected.Timestamp) {
		t.Errorf("Expected timestamp %v, got %v", expected.Timestamp, conv.Timestamp)
	}
	if conv.Duration != expected.Duration {
		t.Errorf("Expected duration %s, got %s", expected.Duration, conv.Duration)
	}
	if conv.Transcript != expected.Transcript {
		t.Errorf("Expected transcript %s, got %s", expected.Transcript, conv.Transcript)
	}
	if len(conv.Participants) != len(expected.Participants) {
		t.Errorf("Expected %d participants, got %d", len(expected.Participants), len(conv.Participants))
	} else {
		for name, number := range expected.Participants {
			if conv.Participants[name] != number {
				t.Errorf("Expected participant %s with number %s, got %s", name, number, conv.Participants[name])
			}
		}
	}
}

func TestParseSMS(t *testing.T) {
	input, err := os.ReadFile("testdata/sms.html")
	if err != nil {
		log.Fatal(err)
	}

	conv, err := parseHTML(string(input))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	expected := Conversation{
		Type: "chat",
		Participants: map[string]string{
			"Me":           "+2222",
			"Tony Smehrik": "+333",
		},
		Messages: []Message{
			{
				Timestamp:    time.Date(2022, 6, 30, 18, 6, 39, 894000000, time.FixedZone("Pacific Time", -7*60*60)),
				Sender:       "Me",
				SenderNumber: "+2222",
				Content:      "doing just fine. I moved to Florida",
			},
			{
				Timestamp:    time.Date(2022, 6, 30, 18, 6, 46, 25000000, time.FixedZone("Pacific Time", -7*60*60)),
				Sender:       "Me",
				SenderNumber: "+2222",
				Content:      "MMS Sent",
				Images:       []string{"Tony Smehrik - Text - 2022-07-01T01_06_39Z-2-1"},
			},
			{
				Timestamp:    time.Date(2022, 6, 30, 18, 7, 9, 468000000, time.FixedZone("Pacific Time", -7*60*60)),
				Sender:       "Tony Smehrik",
				SenderNumber: "+333",
				Content:      "💚",
			},
			{
				Timestamp:    time.Date(2022, 6, 30, 18, 7, 24, 594000000, time.FixedZone("Pacific Time", -7*60*60)),
				Sender:       "Tony Smehrik",
				SenderNumber: "+333",
				Content:      "all that space",
			},
			{
				Timestamp:    time.Date(2022, 6, 30, 18, 7, 28, 190000000, time.FixedZone("Pacific Time", -7*60*60)),
				Sender:       "Tony Smehrik",
				SenderNumber: "+333",
				Content:      "Thank you 🙏",
			},
		},
	}

	if conv.Type != expected.Type {
		t.Errorf("Expected type %s, got %s", expected.Type, conv.Type)
	}
	if len(conv.Participants) != len(expected.Participants) {
		t.Errorf("Expected %d participants, got %d", len(expected.Participants), len(conv.Participants))
		log.Printf("expected=%+v got=%+v", expected.Participants, conv.Participants)
	} else {
		for name, number := range expected.Participants {
			if conv.Participants[name] != number {
				t.Errorf("Expected participant %s with number %s, got %s", name, number, conv.Participants[name])
			}
		}
	}
	if len(conv.Messages) != len(expected.Messages) {
		t.Errorf("Expected %d messages, got %d", len(expected.Messages), len(conv.Messages))
		log.Printf("expected=%+v", expected.Messages)
		log.Printf("got=%+v", conv.Messages)
	} else {
		for i, m := range expected.Messages {
			if !conv.Messages[i].Timestamp.Equal(m.Timestamp) {
				t.Errorf("Message %d: Expected timestamp %v, got %v", i, m.Timestamp, conv.Messages[i].Timestamp)
			}
			if conv.Messages[i].Sender != m.Sender {
				t.Errorf("Message %d: Expected sender %s, got %s", i, m.Sender, conv.Messages[i].Sender)
			}
			if conv.Messages[i].SenderNumber != m.SenderNumber {
				t.Errorf("Message %d: Expected sender number %s, got %s", i, m.SenderNumber, conv.Messages[i].SenderNumber)
			}
			if conv.Messages[i].Content != m.Content {
				t.Errorf("Message %d: Expected content <%s>, got <%s>", i, m.Content, conv.Messages[i].Content)
			}
			if len(conv.Messages[i].Images) != len(m.Images) {
				t.Errorf("Message %d: Expected %d images, got %d", i, len(m.Images), len(conv.Messages[i].Images))
			} else if len(m.Images) > 0 && conv.Messages[i].Images[0] != m.Images[0] {
				t.Errorf("Message %d: Expected image %s, got %s", i, m.Images[0], conv.Messages[i].Images[0])
			}
		}
	}
}

func TestParseGroupMMS(t *testing.T) {
	input, err := os.ReadFile("testdata/mms.html")
	if err != nil {
		log.Fatal(err)
	}

	conv, err := parseHTML(string(input))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	expected := Conversation{
		Type: "chat",
		Participants: map[string]string{
			"Me":           "+2222",
			"Mike Truk":    "+8888",
			"Tony Smehrik": "+333",
		},
		Messages: []Message{
			{
				Timestamp:    time.Date(2024, 5, 22, 21, 48, 32, 703000000, time.FixedZone("Pacific Time", -7*60*60)),
				Sender:       "Mike Truk",
				SenderNumber: "+8888",
				Images:       []string{"Group Conversation - 2024-05-23T04_48_32Z-1-1", "Group Conversation - 2024-05-23T04_48_32Z-1-2"},
			},
			{
				Timestamp:    time.Date(2024, 5, 22, 21, 49, 25, 704000000, time.FixedZone("Pacific Time", -7*60*60)),
				Sender:       "Me",
				SenderNumber: "+2222",
				Images:       []string{"Group Conversation - 2024-05-23T04_48_32Z-2-1"},
			},
			{
				Timestamp:    time.Date(2024, 5, 22, 21, 49, 33, 853000000, time.FixedZone("Pacific Time", -7*60*60)),
				Sender:       "Me",
				SenderNumber: "+2222",
				Images:       []string{"Group Conversation - 2024-05-23T04_48_32Z-3-1"},
			},
			{
				Timestamp:    time.Date(2024, 5, 22, 21, 50, 42, 475000000, time.FixedZone("Pacific Time", -7*60*60)),
				Sender:       "Mike Truk",
				SenderNumber: "+8888",
				Content:      "Hahahaha",
			},
			{
				Timestamp:    time.Date(2024, 5, 22, 21, 51, 10, 663000000, time.FixedZone("Pacific Time", -7*60*60)),
				Sender:       "Mike Truk",
				SenderNumber: "+8888",
				Content:      "Maybe this is your sign to get a hornet-skyscraper Peter",
			},
			{
				Timestamp:    time.Date(2024, 5, 22, 21, 54, 15, 125000000, time.FixedZone("Pacific Time", -7*60*60)),
				Sender:       "Tony Smehrik",
				SenderNumber: "+333",
				Content:      "Hahaha I love all of these",
			},
		},
	}

	if conv.Type != expected.Type {
		t.Errorf("Expected type %s, got %s", expected.Type, conv.Type)
	}
	if len(conv.Participants) != len(expected.Participants) {
		t.Errorf("Expected %d participants, got %d", len(expected.Participants), len(conv.Participants))
		log.Printf("expected=%+v got=%+v", expected.Participants, conv.Participants)
	} else {
		for name, number := range expected.Participants {
			if conv.Participants[name] != number {
				t.Errorf("Expected participant %s with number %s, got %s", name, number, conv.Participants[name])
			}
		}
	}
	if len(conv.Messages) != len(expected.Messages) {
		t.Errorf("Expected %d messages, got %d", len(expected.Messages), len(conv.Messages))
		log.Printf("expected=%+v", expected.Messages)
		log.Printf("got=%+v", conv.Messages)
	} else {
		for i, m := range expected.Messages {
			if !conv.Messages[i].Timestamp.Equal(m.Timestamp) {
				t.Errorf("Message %d: Expected timestamp %v, got %v", i, m.Timestamp, conv.Messages[i].Timestamp)
			}
			if conv.Messages[i].Sender != m.Sender {
				t.Errorf("Message %d: Expected sender %s, got %s", i, m.Sender, conv.Messages[i].Sender)
			}
			if conv.Messages[i].SenderNumber != m.SenderNumber {
				t.Errorf("Message %d: Expected sender number %s, got %s", i, m.SenderNumber, conv.Messages[i].SenderNumber)
			}
			if conv.Messages[i].Content != m.Content {
				t.Errorf("Message %d: Expected content %s, got %s", i, m.Content, conv.Messages[i].Content)
			}
			if len(conv.Messages[i].Images) != len(m.Images) {
				t.Errorf("Message %d: Expected %d images, got %d", i, len(m.Images), len(conv.Messages[i].Images))
			} else {
				for j, img := range m.Images {
					if conv.Messages[i].Images[j] != img {
						t.Errorf("Message %d: Expected image %s, got %s", i, img, conv.Messages[i].Images[j])
					}
				}
			}
		}
	}
}

func TestParseMissedCall(t *testing.T) {
	input, err := os.ReadFile("testdata/missedcall.html")
	if err != nil {
		log.Fatal(err)
	}

	conv, err := parseHTML(string(input))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	expected := Conversation{
		Type: "missed_call",
		Participants: map[string]string{
			"Dwigt Rortugal": "+66666",
		},
		Timestamp: time.Date(2009, 9, 17, 17, 26, 41, 0, time.FixedZone("Pacific Time", -7*60*60)),
	}

	if conv.Type != expected.Type {
		t.Errorf("Expected type %s, got %s", expected.Type, conv.Type)
	}
	if !conv.Timestamp.Equal(expected.Timestamp) {
		t.Errorf("Expected timestamp %v, got %v", expected.Timestamp, conv.Timestamp)
	}
	if len(conv.Participants) != len(expected.Participants) {
		t.Errorf("Expected %d participants, got %d", len(expected.Participants), len(conv.Participants))
	} else {
		for name, number := range expected.Participants {
			if conv.Participants[name] != number {
				t.Errorf("Expected participant %s with number %s, got %s", name, number, conv.Participants[name])
			}
		}
	}
}

func TestParseSMS2(t *testing.T) {
	input, err := os.ReadFile("testdata/sms2.html")
	if err != nil {
		log.Fatal(err)
	}

	conv, err := parseHTML(string(input))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	expected := Conversation{
		Type:      "chat",
		Timestamp: time.Date(2023, 8, 21, 17, 52, 44, 104000000, time.FixedZone("PDT", -7*60*60)),
		Participants: map[string]string{
			"Me":             "+2222",
			"Sillio Sanford": "",
		},
		Messages: []Message{
			{
				Timestamp:    time.Date(2023, 8, 21, 17, 52, 44, 104000000, time.FixedZone("PDT", -7*60*60)),
				Sender:       "Me",
				SenderNumber: "+2222",
				Content:      "Hey ya",
			},
			{
				Timestamp:    time.Date(2023, 8, 21, 18, 2, 19, 924000000, time.FixedZone("PDT", -7*60*60)),
				Sender:       "Me",
				SenderNumber: "+2222",
				Content:      "How are you?",
			},
			{
				Timestamp:    time.Date(2023, 8, 21, 18, 2, 49, 957000000, time.FixedZone("PDT", -7*60*60)),
				Sender:       "Me",
				SenderNumber: "+2222",
				Content:      "Apple",
				Images:       []string{"Sillio Sanford - Text - 2023-08-22T00_52_44Z-3-1"},
			},
			{
				Timestamp:    time.Date(2023, 8, 21, 18, 7, 34, 456000000, time.FixedZone("PDT", -7*60*60)),
				Sender:       "Me",
				SenderNumber: "+2222",
				Content:      "Just text",
			},
			{
				Timestamp:    time.Date(2023, 8, 21, 18, 8, 9, 840000000, time.FixedZone("PDT", -7*60*60)),
				Sender:       "Me",
				SenderNumber: "+2222",
				Content:      "MMS Sent",
				Images:       []string{"Sillio Sanford - Text - 2023-08-22T00_52_44Z-5-1"},
			},
			{
				Timestamp:    time.Date(2023, 8, 21, 21, 12, 17, 519000000, time.FixedZone("PDT", -7*60*60)),
				Sender:       "Me",
				SenderNumber: "+2222",
				Content:      "Hey",
			},
		},
	}

	if conv.Type != expected.Type {
		t.Errorf("Expected type %s, got %s", expected.Type, conv.Type)
	}
	if !conv.Timestamp.Equal(expected.Timestamp) {
		t.Errorf("Expected type %s, got %s", expected.Timestamp, conv.Timestamp)
	}

	if len(conv.Participants) != len(expected.Participants) {
		t.Errorf("Expected %d participants, got %d", len(expected.Participants), len(conv.Participants))
	} else {
		for name, number := range expected.Participants {
			if conv.Participants[name] != number {
				t.Errorf("Expected participant %s with number %s, got %s", name, number, conv.Participants[name])
			}
		}
	}
	if len(conv.Messages) != len(expected.Messages) {
		t.Errorf("Expected %d messages, got %d", len(expected.Messages), len(conv.Messages))
	} else {
		for i, m := range expected.Messages {
			if !conv.Messages[i].Timestamp.Equal(m.Timestamp) {
				t.Errorf("Message %d: Expected timestamp %v, got %v", i, m.Timestamp, conv.Messages[i].Timestamp)
			}
			if conv.Messages[i].Sender != m.Sender {
				t.Errorf("Message %d: Expected sender %s, got %s", i, m.Sender, conv.Messages[i].Sender)
			}
			if conv.Messages[i].SenderNumber != m.SenderNumber {
				t.Errorf("Message %d: Expected sender number %s, got %s", i, m.SenderNumber, conv.Messages[i].SenderNumber)
			}
			if conv.Messages[i].Content != m.Content {
				t.Errorf("Message %d: Expected content %s, got %s", i, m.Content, conv.Messages[i].Content)
			}
			if len(conv.Messages[i].Images) != len(m.Images) {
				t.Errorf("Message %d: Expected %d images, got %d", i, len(m.Images), len(conv.Messages[i].Images))
			} else {
				for j, img := range m.Images {
					if conv.Messages[i].Images[j] != img {
						t.Errorf("Message %d: Expected image %s, got %s", i, img, conv.Messages[i].Images[j])
					}
				}
			}
		}
	}
}
