package traffic

import (
	"testing"
	"time"
)

func TestRealByteWindowsAndUnavailableFirstSample(t *testing.T) {
	c := New()
	start := c.at
	c.Add(1000)
	c.Add(-20)
	if c.rateAt(start.Add(500*time.Millisecond)) != nil {
		t.Fatal("rate invented before observation window")
	}
	if rate := c.rateAt(start.Add(2 * time.Second)); rate == nil || *rate != 4000 {
		t.Fatal("incorrect measured rate", rate)
	}
	if rate := c.rateAt(start.Add(4 * time.Second)); rate == nil || *rate != 0 {
		t.Fatal("idle measured window should be zero")
	}
	if c.Bytes() != 1000 {
		t.Fatal("incorrect payload count")
	}
}
