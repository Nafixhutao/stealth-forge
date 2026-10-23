package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Nafixhutao/stealth-forge/internal/domain"
)

// A route mounted without its middleware is a wiring bug, not an
// unauthenticated caller. The accessor must still fail loudly, but the message
// has to name the middleware that is missing: the bare interface-conversion
// panic it replaces said only "interface {} is nil, not domain.Account", which
// reads like a caller error and sends the reader to the wrong place.
func TestRequestValuePanicsNamingTheMissingMiddleware(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/v1/account", nil)

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected a panic when the middleware never ran")
		}
		message, _ := recovered.(string)
		if !strings.Contains(message, "requireSession") {
			t.Fatalf("panic %q does not name the middleware to mount", message)
		}
		if !strings.Contains(message, "account") {
			t.Fatalf("panic %q does not name the context key", message)
		}
	}()

	_ = requestValue[domain.Account](request, accountContextKey, "requireSession")
}

// The happy path must return the stored value unchanged, so the accessors stay
// a drop-in replacement for the type assertions they replaced.
func TestRequestValueReturnsTheStoredValue(t *testing.T) {
	want := domain.Account{ID: "01a1648a-0000-7000-8000-000000000001"}
	request := httptest.NewRequest(http.MethodGet, "/v1/account", nil)
	request = request.WithContext(
		context.WithValue(request.Context(), accountContextKey, want),
	)

	got := requestValue[domain.Account](request, accountContextKey, "requireSession")
	if got.ID != want.ID {
		t.Fatalf("account id = %q, want %q", got.ID, want.ID)
	}
}
