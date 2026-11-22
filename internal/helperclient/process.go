package helperclient

import (
	"context"
	"errors"
	"time"

	"tunnelkit/internal/privilege"
)

// WaitForExit confirms process termination. A stop acknowledgement alone is not
// proof of exit, and an access-denied process probe is never treated as absence.
func (c *Client) WaitForExit(ctx context.Context) error {
	if c.pid <= 0 {
		return errors.New("helper process identity is unavailable")
	}
	if c.processExit != nil {
		if err, decided := c.processExit.wait(ctx); decided {
			return err
		}
	}
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		present, err := privilege.IsProcessAlive(c.pid)
		if err != nil {
			return err
		}
		if !present {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
