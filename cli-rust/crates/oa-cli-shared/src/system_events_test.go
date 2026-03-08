package infra

import (
	"testing"
)

func TestSystemEventQueue_EnqueueDrain(t *testing.T) {
	q := NewSystemEventQueue()
	q.Enqueue("hello", "s1", "")
	q.Enqueue("world", "s1", "")

	events := q.Drain("s1")
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Text != "hello" || events[1].Text != "world" {
		t.Fatal("wrong event texts")
	}
	// Drain 后应为空
	if q.Has("s1") {
		t.Fatal("expected empty after drain")
	}
}

func TestSystemEventQueue_Dedup(t *testing.T) {
	q := NewSystemEventQueue()
	q.Enqueue("same", "s1", "")
	q.Enqueue("same", "s1", "")
	q.Enqueue("same", "s1", "")

	events := q.Drain("s1")
	if len(events) != 1 {
		t.Fatalf("expected 1 event after dedup, got %d", len(events))
	}
}

func TestSystemEventQueue_MaxEvents(t *testing.T) {
	q := NewSystemEventQueue()
	for i := 0; i < 25; i++ {
		q.Enqueue("evt-"+string(rune('A'+i)), "s1", "")
	}
	events := q.Drain("s1")
	if len(events) != maxSystemEvents {
		t.Fatalf("expected %d events (capped), got %d", maxSystemEvents, len(events))
	}
}

func TestSystemEventQueue_InvalidSessionKey(t *testing.T) {
	q := NewSystemEventQueue()
	q.Enqueue("test", "", "")
	q.Enqueue("test", "  ", "")
	if q.Has("") {
		t.Fatal("expected no events for empty key")
	}
}

func TestSystemEventQueue_Peek(t *testing.T) {
	q := NewSystemEventQueue()
	q.Enqueue("peek-test", "s1", "")
	texts := q.Peek("s1")
	if len(texts) != 1 || texts[0] != "peek-test" {
		t.Fatal("peek failed")
	}
	// Peek 不消费
	if !q.Has("s1") {
		t.Fatal("expected still has events after peek")
	}
}

func TestSystemEventQueue_DrainTexts(t *testing.T) {
	q := NewSystemEventQueue()
	q.Enqueue("a", "s1", "")
	q.Enqueue("b", "s1", "")
	texts := q.DrainTexts("s1")
	if len(texts) != 2 || texts[0] != "a" || texts[1] != "b" {
		t.Fatal("drain texts failed")
	}
}

func TestSystemEventQueue_ContextChanged(t *testing.T) {
	q := NewSystemEventQueue()
	// 新 session，任何上下文都算变更
	if !q.IsContextChanged("s1", "ctx1") {
		t.Fatal("new session should show context changed")
	}
	q.Enqueue("evt", "s1", "ctx1")
	if q.IsContextChanged("s1", "ctx1") {
		t.Fatal("same context should not be changed")
	}
	if !q.IsContextChanged("s1", "ctx2") {
		t.Fatal("different context should be changed")
	}
}
