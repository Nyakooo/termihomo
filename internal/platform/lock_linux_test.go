package platform

import (
	"errors"
	"testing"
)

func TestInstanceLockCanBeReacquiredAfterClose(t *testing.T) {
	p := Paths{Data: t.TempDir()}
	first, err := p.LockInstance()
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := p.LockInstance()
	if second != nil {
		second.Close()
	}
	if !errors.Is(err, ErrInstanceBusy) {
		t.Fatalf("concurrent lock = %v; want ErrInstanceBusy", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := p.LockInstance()
	if err != nil {
		t.Fatalf("lock remained held after close: %v", err)
	}
	defer next.Close()
}
