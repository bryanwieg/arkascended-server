package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pkg/ark"
	"pkg/ark/arktest"
	"pkg/ark/health"
	"pkg/ark/lifecycle"

	"github.com/compose-spec/compose-go/v2/types"
)

func fakeProject(services ...string) *ark.ComposeProject {
	svcMap := make(types.Services)
	for _, name := range services {
		svcMap[name] = types.ServiceConfig{Name: name}
	}
	return &ark.ComposeProject{Project: &types.Project{Services: svcMap}}
}

func testDeps() *Deps {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mock := &arktest.MockCompose{}
	hc := health.New(mock, logger)
	lc := lifecycle.New(mock, hc, logger)
	return &Deps{
		Engine:         &arktest.MockEngine{},
		ComposeService: mock,
		HealthChecker:  hc,
		Lifecycle:      lc,
		Logger:         logger,
		FindProjects:   func() ([]string, error) { return nil, nil },
	}
}

func TestHandleHealth(t *testing.T) {
	srv := New(testDeps())
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("expected status=ok, got %q", body["status"])
	}
}

func TestHandleHomeNoServices(t *testing.T) {
	deps := testDeps()
	deps.FindProjects = func() ([]string, error) { return []string{"compose.yaml"}, nil }
	deps.ComposeService = &arktest.MockCompose{
		RunningServicesFn: func(ctx context.Context, composeFile string) (map[string]struct{}, error) {
			return map[string]struct{}{}, nil
		},
	}

	srv := New(deps)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "No instances running") {
		t.Fatalf("expected 'No instances running' in body:\n%s", body)
	}
}

func TestHandleHomeWithRunning(t *testing.T) {
	deps := testDeps()
	deps.FindProjects = func() ([]string, error) { return []string{"compose.yaml"}, nil }
	deps.ComposeService = &arktest.MockCompose{
		RunningServicesFn: func(ctx context.Context, composeFile string) (map[string]struct{}, error) {
			return map[string]struct{}{"asa1": {}, "asa2": {}}, nil
		},
	}

	srv := New(deps)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "2 instance(s) running") {
		t.Fatalf("expected '2 instance(s) running' in body:\n%s", body)
	}
	if !strings.Contains(body, "asa1") || !strings.Contains(body, "asa2") {
		t.Fatalf("expected instance names in body:\n%s", body)
	}
}

func TestHandleListInstances(t *testing.T) {
	deps := testDeps()
	deps.FindProjects = func() ([]string, error) { return []string{"compose.yaml"}, nil }
	deps.ComposeService = &arktest.MockCompose{
		RunningServicesFn: func(ctx context.Context, composeFile string) (map[string]struct{}, error) {
			return map[string]struct{}{"asa1": {}}, nil
		},
		ExitedServicesFn: func(ctx context.Context, composeFile string) (map[string]struct{}, error) {
			return map[string]struct{}{"asa2": {}}, nil
		},
		AllServicesFn: func(ctx context.Context, composeFile string) (map[string]struct{}, error) {
			return map[string]struct{}{"asa1": {}, "asa2": {}}, nil
		},
	}

	srv := New(deps)
	req := httptest.NewRequest("GET", "/instances", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "running") {
		t.Fatalf("expected 'running' in body:\n%s", body)
	}
	if !strings.Contains(body, "exited") {
		t.Fatalf("expected 'exited' in body:\n%s", body)
	}
}

func TestHandleGetInstanceRunning(t *testing.T) {
	deps := testDeps()
	deps.FindProjects = func() ([]string, error) { return []string{"compose.yaml"}, nil }
	deps.ComposeService = &arktest.MockCompose{
		RunningServicesFn: func(ctx context.Context, composeFile string) (map[string]struct{}, error) {
			return map[string]struct{}{"asa1": {}}, nil
		},
		ExitedServicesFn: func(ctx context.Context, composeFile string) (map[string]struct{}, error) {
			return map[string]struct{}{}, nil
		},
	}

	srv := New(deps)
	req := httptest.NewRequest("GET", "/instances/asa1", nil)
	req.SetPathValue("instance", "asa1")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "asa1") {
		t.Fatalf("expected instance name in body:\n%s", body)
	}
	if !strings.Contains(body, "Stop") {
		t.Fatalf("expected Stop button in body:\n%s", body)
	}
}

func TestHandleViewCompose(t *testing.T) {
	dir := t.TempDir()
	composeFile := filepath.Join(dir, "docker-compose.yaml")
	if err := os.WriteFile(composeFile, []byte("services:\n  asa1:\n    image: test\n"), 0644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	deps := testDeps()
	deps.FindProjects = func() ([]string, error) { return []string{composeFile}, nil }
	deps.ComposeService = &arktest.MockCompose{
		LoadProjectFn: func(ctx context.Context, composeFile string) (*ark.ComposeProject, error) {
			return fakeProject("asa1"), nil
		},
	}

	srv := New(deps)
	req := httptest.NewRequest("GET", "/instances/asa1/compose", nil)
	req.SetPathValue("instance", "asa1")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "services:") {
		t.Fatalf("expected compose content in body:\n%s", body)
	}
	if !strings.Contains(body, "Save") {
		t.Fatalf("expected Save button in body:\n%s", body)
	}
}

func TestHandleEditComposeValidationError(t *testing.T) {
	dir := t.TempDir()
	composeFile := filepath.Join(dir, "docker-compose.yaml")
	if err := os.WriteFile(composeFile, []byte("services:\n  asa1:\n    image: test\n"), 0644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	deps := testDeps()
	deps.FindProjects = func() ([]string, error) { return []string{composeFile}, nil }
	deps.ComposeService = &arktest.MockCompose{
		LoadProjectFn: func(ctx context.Context, path string) (*ark.ComposeProject, error) {
			if strings.Contains(path, "validate") {
				return nil, ark.ErrNotFound
			}
			return fakeProject("asa1"), nil
		},
	}

	srv := New(deps)
	form := strings.NewReader("content=invalid: [yaml")
	req := httptest.NewRequest("POST", "/instances/asa1/compose", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("instance", "asa1")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (form re-rendered), got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Invalid compose YAML") {
		t.Fatalf("expected validation error in body:\n%s", body)
	}
}

func TestHandleEditComposeSave(t *testing.T) {
	dir := t.TempDir()
	composeFile := filepath.Join(dir, "docker-compose.yaml")
	if err := os.WriteFile(composeFile, []byte("services:\n  asa1:\n    image: old\n"), 0644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	deps := testDeps()
	deps.FindProjects = func() ([]string, error) { return []string{composeFile}, nil }
	deps.ComposeService = &arktest.MockCompose{
		LoadProjectFn: func(ctx context.Context, path string) (*ark.ComposeProject, error) {
			return fakeProject("asa1"), nil
		},
	}

	srv := New(deps)
	newContent := "services:\n  asa1:\n    image: new\n"
	form := strings.NewReader("content=" + newContent)
	req := httptest.NewRequest("POST", "/instances/asa1/compose", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("instance", "asa1")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Saved successfully") {
		t.Fatalf("expected 'Saved successfully' in body:\n%s", body)
	}

	written, err := os.ReadFile(composeFile)
	if err != nil {
		t.Fatalf("read saved compose: %v", err)
	}
	if string(written) != newContent {
		t.Fatalf("expected %q on disk, got %q", newContent, string(written))
	}
}
