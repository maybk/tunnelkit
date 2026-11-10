package helperclient

import (
	"testing"

	"tunnelkit/fortivpn"
)

func TestControlMessagesKeepOrderWhenStatisticsQueueIsFull(t *testing.T) {
	c := &Client{eventsCh: make(chan Message, 3)}
	waiting := Message{Event: &fortivpn.Event{Phase: fortivpn.PhaseWaitingInput}}
	stats := Message{Event: &fortivpn.Event{Phase: fortivpn.PhaseConnected, Stats: &fortivpn.Stats{}}}
	prompt := Message{Prompt: &fortivpn.Prompt{ID: "active", Kind: fortivpn.PromptOTP}}
	connected := Message{Event: &fortivpn.Event{Phase: fortivpn.PhaseConnected}}
	c.pushMessage(waiting)
	c.pushMessage(stats)
	c.pushMessage(prompt)
	if !c.pushMessage(connected) {
		t.Fatal("replaceable statistics blocked a phase change")
	}
	if (<-c.eventsCh).Event.Phase != fortivpn.PhaseWaitingInput || (<-c.eventsCh).Prompt.ID != "active" || (<-c.eventsCh).Event.Phase != fortivpn.PhaseConnected {
		t.Fatal("control message order changed")
	}
}

func TestControlOnlyQueueReportsLossInsteadOfSilentlyEvicting(t *testing.T) {
	c := &Client{eventsCh: make(chan Message, 1)}
	c.pushMessage(Message{Event: &fortivpn.Event{Phase: fortivpn.PhaseWaitingInput}})
	if c.pushMessage(Message{Event: &fortivpn.Event{Phase: fortivpn.PhaseConnected}}) {
		t.Fatal("control overflow was not reported")
	}
	if (<-c.eventsCh).Event.Phase != fortivpn.PhaseWaitingInput {
		t.Fatal("old control message was silently evicted")
	}
}
