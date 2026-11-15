package core

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRulesPreserveControllerOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/rules" || r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"rules":[{"index":0,"type":"Domain","payload":"example.org","proxy":"DIRECT","size":-1},{"index":1,"type":"Match","payload":"","proxy":"Proxy","size":-1}]}`)
	}))
	defer server.Close()
	state := instance{Controller: strings.TrimPrefix(server.URL, "http://"), Secret: "test-secret"}
	rules, err := state.rules(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 || rules[0].Payload != "example.org" || rules[1].Proxy != "Proxy" || rules[1].Index != 1 {
		t.Fatalf("unexpected rules: %+v", rules)
	}
}

func TestRulesReportsControllerFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	state := instance{Controller: strings.TrimPrefix(server.URL, "http://")}
	if _, err := state.rules(context.Background()); err == nil {
		t.Fatal("expected controller error")
	}
}
