package core

// AppDepsRuntimeState manages the currently active AppDeps instance.
type AppDepsRuntimeState struct {
	Active *AppDeps
}

// GlobalAppDepsState holds the singleton active AppDeps.
var GlobalAppDepsState = &AppDepsRuntimeState{Active: &AppDeps{}}

// CurrentAppDeps returns the currently active AppDeps.
func CurrentAppDeps() *AppDeps {
	return GlobalAppDepsState.Active
}

// SetCurrentAppDeps sets the currently active AppDeps.
func SetCurrentAppDeps(deps *AppDeps) {
	GlobalAppDepsState.Active = deps
}

// WithAppDeps runs fn with temp as the active AppDeps, restoring the previous
// value afterward. If fn is nil, WithAppDeps is a no-op. If temp is nil,
// a zero-value AppDeps is used.
func WithAppDeps(temp *AppDeps, fn func()) {
	if fn == nil {
		return
	}
	if temp == nil {
		temp = &AppDeps{}
	}
	prev := CurrentAppDeps()
	SetCurrentAppDeps(temp)
	defer SetCurrentAppDeps(prev)
	fn()
}

// WithAppDepsErr runs fn with temp as the active AppDeps, returning its error.
func WithAppDepsErr(temp *AppDeps, fn func() error) error {
	if fn == nil {
		return nil
	}
	var err error
	WithAppDeps(temp, func() {
		err = fn()
	})
	return err
}
