package wgbridge

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerRejectsUnconfirmedAndArbitraryCommands(t *testing.T) {
	e, k, _ := fake(t)
	handler := Handler(e)
	for _, body := range []string{`{"confirmed":false}`, `{"confirmed":true,"command":"reboot"}`, `{"confirmed":true} {"extra":true}`, `broken`} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("POST", "/apply", strings.NewReader(body)))
		if response.Code != 400 {
			t.Fatal("unsafe input accepted", response.Code)
		}
	}
	if len(k.calls) != 0 {
		t.Fatal("invalid request reached runner")
	}
}

func TestInterruptedJournalBlocksApplyWithoutNetworkMutation(t *testing.T) {
	e, k, c := fake(t)
	e.record.Pending = c
	if err := e.save(); err != nil {
		t.Fatal(err)
	}
	recovered, err := New(e.directory, e.load, e.run)
	if err != nil {
		t.Fatal(err)
	}
	if status := recovered.Status(context.Background()); status.Stage != "recovery_required" || status.Applied {
		t.Fatal("interruption hidden")
	}
	if _, err = recovered.Apply(strings.Repeat("a", 64)); err != ErrConflict {
		t.Fatal("interrupted operation accepted")
	}
	if len(k.calls) != 0 {
		t.Fatal("startup mutated network")
	}
}
