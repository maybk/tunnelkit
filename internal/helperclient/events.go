package helperclient

import "tunnelkit/fortivpn"

func replaceableMessage(m Message) bool {
	if m.LogLine != "" {
		return true
	}
	e := m.Event
	return e != nil && e.Phase == fortivpn.PhaseConnected && e.Stats != nil && e.Info == nil && e.Prompt == nil && e.Err == nil
}

// pushMessage evicts only replaceable stats/logs. A false result terminates the
// control stream instead of silently losing a lifecycle transition.
func (c *Client) pushMessage(m Message) bool {
	c.eventsMu.Lock()
	defer c.eventsMu.Unlock()
	select {
	case c.eventsCh <- m:
		return true
	default:
	}
	if replaceableMessage(m) {
		return true
	}
	queued := make([]Message, 0, cap(c.eventsCh))
drain:
	for {
		select {
		case item := <-c.eventsCh:
			queued = append(queued, item)
		default:
			break drain
		}
	}
	if len(queued) == cap(c.eventsCh) {
		drop := -1
		for i, item := range queued {
			if replaceableMessage(item) {
				drop = i
				break
			}
		}
		if drop == -1 {
			for _, item := range queued {
				c.eventsCh <- item
			}
			return false
		}
		queued = append(queued[:drop], queued[drop+1:]...)
	}
	for _, item := range queued {
		c.eventsCh <- item
	}
	c.eventsCh <- m
	return true
}
