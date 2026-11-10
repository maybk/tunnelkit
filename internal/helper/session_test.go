package helper

import (
	"context"
	"errors"
	"testing"
	"time"

	"tunnelkit/fortivpn"
)

func TestPromptCancellationRevokesAnswerability(t *testing.T) {
	s := newSessionState(time.Second)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	done := make(chan error, 1)
	go func() {
		_, err := s.Prompt(ctx, fortivpn.Prompt{ID: "active", Kind: fortivpn.PromptOTP})
		done <- err
	}()
	deadline := time.After(time.Second)
	for {
		s.mu.Lock()
		ready := s.pendingPrompt != nil
		s.mu.Unlock()
		if ready {
			break
		}
		select {
		case <-deadline:
			t.Fatal("prompt did not start")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	if err := s.DeliverAnswer("active", "", true); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("user cancellation must be distinguishable, got %v", err)
	}
	s.mu.Lock()
	snapshot := s.snapshotLocked()
	s.mu.Unlock()
	if snapshot.Prompt != nil {
		t.Fatal("cancelled prompt remained in snapshot")
	}
}

func TestPhaseAdvanceRevokesPendingPrompt(t *testing.T) {
	s := newSessionState(time.Second)
	s.lastPhase = fortivpn.PhaseWaitingInput
	s.pendingPrompt = &fortivpn.Prompt{ID: "old", Kind: fortivpn.PromptOTP}
	s.promptCh = make(chan promptResponse, 1)
	s.OnEvent(fortivpn.Event{Phase: fortivpn.PhaseConnected})
	s.mu.Lock()
	snapshot := s.snapshotLocked()
	s.mu.Unlock()
	if snapshot.Prompt != nil || s.DeliverAnswer("old", "synthetic", false) == nil {
		t.Fatal("phase advance left an answerable challenge")
	}
}
