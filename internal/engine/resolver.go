package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/shanecandoit/ProcessStarlark/internal/samples"
	"go.starlark.net/starlark"
)

// ErrUnknown is what a Resolver returns when it has no answer, so the next
// resolver (or, after the last, a question) gets the call.
var ErrUnknown = errors.New("no answer")

// Call is a call that no example, cached result, or code could answer.
type Call struct {
	Function string
	Inputs   []string       // Input names in signature order, for display.
	Input    map[string]any // Named inputs.
}

// Resolver answers calls nothing in the project can: a model, a default, an
// external program. Its answers are always proposals, like unconfirmed code:
// returned for the run, queued for a person to confirm, never cached, and never
// counted as examples. Resolvers are not consulted by park test.
type Resolver interface {
	// Name labels the resolver's answers, e.g. in --why.
	Name() string
	// Resolve returns an answer, ErrUnknown to pass, or another error to fail
	// the run.
	Resolve(ctx context.Context, call Call) (any, error)
}

const contextKey = "park.context"

// threadContext returns the context Run stored on the thread.
func threadContext(thread *starlark.Thread) context.Context {
	if ctx, ok := thread.Local(contextKey).(context.Context); ok {
		return ctx
	}
	return context.Background()
}

// ask offers a call to each resolver in turn. ok is false when none answered.
func (e *Engine) ask(thread *starlark.Thread, call Call) (out any, by string, ok bool, err error) {
	if e.Testing {
		return nil, "", false, nil
	}
	ctx := threadContext(thread)
	for _, r := range e.Resolvers {
		if err := ctx.Err(); err != nil {
			return nil, "", false, err
		}
		answer, err := r.Resolve(ctx, call)
		if errors.Is(err, ErrUnknown) {
			continue
		}
		if err != nil {
			return nil, "", false, fmt.Errorf("resolver %s for %s: %w", r.Name(), FormatCall(call.Function, call.Inputs, call.Input), err)
		}
		// Normalize through canonical JSON so a resolver's Go values become
		// Park values (int -> int64, and so on) or fail clearly.
		b, err := samples.Canonical(answer)
		if err != nil {
			return nil, "", false, fmt.Errorf("resolver %s answer for %s: %w", r.Name(), call.Function, err)
		}
		value, err := samples.ParseJSON(b)
		if err != nil {
			return nil, "", false, fmt.Errorf("resolver %s answer for %s: %w", r.Name(), call.Function, err)
		}
		return value, r.Name(), true, nil
	}
	return nil, "", false, nil
}
