package httpapi

import (
	"fmt"
	"net/http"
)

// requestValue reads a typed value that a middleware placed on the request
// context. It exists so a route mounted without its middleware fails with a
// message that names the missing middleware, instead of Go's bare
// "interface conversion: interface {} is nil, not domain.Account".
//
// The distinction matters for diagnosis, not for the response. A handler
// reaching this without its middleware is a server wiring bug, so the
// recovery middleware's 500 is the honest status: the caller is not
// unauthenticated, the route is simply mounted wrong. What was missing was the
// log line that says which middleware to add.
func requestValue[T any](
	r *http.Request,
	key contextKey,
	middleware string,
) T {
	raw := r.Context().Value(key)
	value, ok := raw.(T)
	if !ok {
		// Report the type that was actually stored, not the type the caller
		// wanted: "nil" means the middleware never ran, while a named type
		// means the wrong middleware did.
		panic(fmt.Sprintf(
			"httpapi: route reached a handler that needs %s, but context key %q holds %T instead of %T; mount the handler behind %s",
			middleware,
			string(key),
			raw,
			value,
			middleware,
		))
	}
	return value
}
