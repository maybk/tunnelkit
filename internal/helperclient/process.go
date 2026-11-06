package helperclient

import (
	"context"
	"errors"
	"time"
)

// WaitForExit confirms process termination. A stop acknowledgement alone is not
// proof of exit, and an access-denied process probe is never treated as absence.
func (c *Client) WaitForExit(ctx context.Context) error {
	if c.pid <= 0 {
		return errors.New("helper process identity is unavailable")
	}
	if c.processExit != nil {
		select {
		case err := <-c.processExit:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		present, err := processPresent(c.pid)
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
