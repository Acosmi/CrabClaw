package nativemsg

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	msg := []byte(`{"type":"list_tabs"}`)
	var buf bytes.Buffer

	if err := WriteMessage(&buf, msg); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	// Verify length prefix.
	if buf.Len() != 4+len(msg) {
		t.Fatalf("expected %d bytes, got %d", 4+len(msg), buf.Len())
	}

	got, err := ReadMessage(&buf)
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("payload mismatch: got %q, want %q", got, msg)
	}
}

func TestReadMessage_TooLarge(t *testing.T) {
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, uint32(MaxMessageSize+1))
	buf.Write(make([]byte, 8)) // partial payload

	_, err := ReadMessage(&buf)
	if err == nil {
		t.Fatal("expected error for oversized message")
	}
}

func TestReadMessage_Empty(t *testing.T) {
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, uint32(0))

	_, err := ReadMessage(&buf)
	if err == nil {
		t.Fatal("expected error for empty message")
	}
}

func TestWriteMessage_TooLarge(t *testing.T) {
	var buf bytes.Buffer
	err := WriteMessage(&buf, make([]byte, MaxMessageSize+1))
	if err == nil {
		t.Fatal("expected error for oversized message")
	}
}

func TestMultipleMessages(t *testing.T) {
	messages := []string{
		`{"type":"ping"}`,
		`{"type":"tab_list","tabs":[]}`,
		`{"type":"cdp","method":"Page.navigate","params":{"url":"https://example.com"}}`,
	}

	var buf bytes.Buffer
	for _, m := range messages {
		if err := WriteMessage(&buf, []byte(m)); err != nil {
			t.Fatalf("WriteMessage(%s): %v", m, err)
		}
	}

	for _, want := range messages {
		got, err := ReadMessage(&buf)
		if err != nil {
			t.Fatalf("ReadMessage: %v", err)
		}
		if string(got) != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
}
