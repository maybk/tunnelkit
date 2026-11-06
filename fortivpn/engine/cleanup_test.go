package engine

import (
	"context"
	"errors"
	"testing"

	"tunnelkit/fortivpn"
	"tunnelkit/fortivpn/internal/netcfg"
)

type failingNetworkCleanup struct{ err error }

func (f failingNetworkCleanup) Update(context.Context, netcfg.Desired) error { return nil }
func (f failingNetworkCleanup) Close(context.Context) error                  { return f.err }

func TestSessionReportsIncompleteNetworkCleanup(t *testing.T) {
	failure := errors.New("network service unavailable")
	s := newSession(fortivpn.Config{}, nil, defaultOptions(), false)
	s.applied = failingNetworkCleanup{failure}
	if err := s.teardown(nil, ""); !errors.Is(err, failure) {
		t.Fatalf("lost cleanup failure: %v", err)
	}
}
