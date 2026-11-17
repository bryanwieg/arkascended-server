package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"pkg/ark"
	"pkg/ark/backup"
	"pkg/ark/health"
	"pkg/ark/lifecycle"
	"pkg/ark/runtime"
	"pkg/ark/schedule"
	"pkg/ark/update"
)

type Deps struct {
	Engine          ark.ContainerEngine
	ComposeService  ark.ComposeService
	HealthChecker   health.InstanceHealthChecker
	Lifecycle       *lifecycle.Service
	BackupService   *backup.Service
	UpdateService   *update.Service
	ScheduleService *schedule.Service
	Runtime         runtime.RuntimeDeps
	Logger          *slog.Logger

	FindProjects func() ([]string, error)
}

type Server struct {
	deps   *Deps
	mux    *http.ServeMux
	logger *slog.Logger
	render *renderer
}

func New(deps *Deps) *Server {
	s := &Server{
		deps:   deps,
		mux:    http.NewServeMux(),
		logger: deps.Logger,
		render: newRenderer(),
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	s.mux.Handle("GET /static/", serveStatic())

	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /", s.handleHome)
	s.mux.HandleFunc("GET /instances", s.handleListInstances)
	s.mux.HandleFunc("GET /instances/{instance}", s.handleGetInstance)
	s.mux.HandleFunc("POST /instances/{instance}/start", s.handleStartInstance)
	s.mux.HandleFunc("POST /instances/{instance}/stop", s.handleStopInstance)
	s.mux.HandleFunc("POST /instances/{instance}/restart", s.handleRestartInstance)
	s.mux.HandleFunc("GET /instances/{instance}/compose", s.handleViewCompose)
	s.mux.HandleFunc("POST /instances/{instance}/compose", s.handleEditCompose)
	s.mux.HandleFunc("GET /instances/{instance}/ini", s.handleViewINI)
	s.mux.HandleFunc("POST /instances/{instance}/ini", s.handleEditINI)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	type homeData struct {
		Error           string
		RunningServices []string
	}

	data := homeData{}
	if s.deps.ComposeService != nil {
		projects, err := s.findProjects()
		if err != nil {
			data.Error = err.Error()
		} else {
			for _, p := range projects {
				running, _ := s.deps.ComposeService.RunningServices(r.Context(), p)
				for svc := range running {
					data.RunningServices = append(data.RunningServices, svc)
				}
			}
		}
	}

	if isHTMX(r) {
		s.render.renderPartial(w, "home", "content", data)
		return
	}
	s.render.render(w, "home", data)
}

func (s *Server) handleListInstances(w http.ResponseWriter, r *http.Request) {
	type serviceInfo struct {
		Name   string
		Status string
	}

	type listData struct {
		Error    string
		Services []serviceInfo
	}

	data := listData{}
	if s.deps.ComposeService == nil {
		data.Error = "ComposeService not available"
	} else {
		projects, err := s.findProjects()
		if err != nil {
			data.Error = err.Error()
		} else {
			for _, p := range projects {
				running, _ := s.deps.ComposeService.RunningServices(r.Context(), p)
				exited, _ := s.deps.ComposeService.ExitedServices(r.Context(), p)
				all, _ := s.deps.ComposeService.AllServices(r.Context(), p)
				for svc := range all {
					status := "unknown"
					if _, ok := running[svc]; ok {
						status = "running"
					} else if _, ok := exited[svc]; ok {
						status = "exited"
					}
					data.Services = append(data.Services, serviceInfo{Name: svc, Status: status})
				}
			}
		}
	}

	if isHTMX(r) {
		s.render.renderPartial(w, "instances", "content", data)
		return
	}
	s.render.render(w, "instances", data)
}

func (s *Server) handleGetInstance(w http.ResponseWriter, r *http.Request) {
	instance := r.PathValue("instance")

	type instanceData struct {
		Name   string
		Status string
		Error  string
	}

	data := instanceData{Name: instance, Status: "unknown"}

	if s.deps.ComposeService != nil {
		projects, err := s.findProjects()
		if err != nil {
			data.Error = err.Error()
		} else {
			for _, p := range projects {
				running, _ := s.deps.ComposeService.RunningServices(r.Context(), p)
				if _, ok := running[instance]; ok {
					data.Status = "running"
					break
				}
				exited, _ := s.deps.ComposeService.ExitedServices(r.Context(), p)
				if _, ok := exited[instance]; ok {
					data.Status = "exited"
					break
				}
			}
		}
	}

	if isHTMX(r) {
		s.render.renderPartial(w, "instance", "content", data)
		return
	}
	s.render.render(w, "instance", data)
}

func (s *Server) handleStartInstance(w http.ResponseWriter, r *http.Request) {
	instance := r.PathValue("instance")

	type instanceData struct {
		Name   string
		Status string
		Error  string
	}

	data := instanceData{Name: instance, Status: "running"}

	if s.deps.Lifecycle != nil {
		project, err := s.findProjectForService(instance)
		if err != nil {
			data.Error = err.Error()
			data.Status = "unknown"
		} else {
			startReq := lifecycle.StartRequest{
				ComposeFile: project,
				Services:    []string{instance},
			}
			if _, err := s.deps.Lifecycle.Start(r.Context(), startReq); err != nil {
				data.Error = err.Error()
				data.Status = "exited"
			}
		}
	} else {
		data.Error = "Lifecycle service not available"
	}

	s.render.renderPartial(w, "instance", "content", data)
}

func (s *Server) handleStopInstance(w http.ResponseWriter, r *http.Request) {
	instance := r.PathValue("instance")

	type instanceData struct {
		Name   string
		Status string
		Error  string
	}

	data := instanceData{Name: instance, Status: "exited"}

	if s.deps.Lifecycle != nil {
		project, err := s.findProjectForService(instance)
		if err != nil {
			data.Error = err.Error()
			data.Status = "unknown"
		} else {
			stopReq := lifecycle.StopRequest{
				ComposeFile: project,
				Services:    []string{instance},
			}
			if err := s.deps.Lifecycle.Stop(r.Context(), stopReq); err != nil {
				data.Error = err.Error()
				data.Status = "running"
			}
		}
	} else {
		data.Error = "Lifecycle service not available"
	}

	s.render.renderPartial(w, "instance", "content", data)
}

func (s *Server) handleRestartInstance(w http.ResponseWriter, r *http.Request) {
	instance := r.PathValue("instance")

	type instanceData struct {
		Name   string
		Status string
		Error  string
	}

	data := instanceData{Name: instance, Status: "running"}

	if s.deps.Lifecycle != nil {
		project, err := s.findProjectForService(instance)
		if err != nil {
			data.Error = err.Error()
			data.Status = "unknown"
		} else {
			stopReq := lifecycle.StopRequest{
				ComposeFile: project,
				Services:    []string{instance},
			}
			if err := s.deps.Lifecycle.Stop(r.Context(), stopReq); err != nil {
				data.Error = err.Error()
				data.Status = "exited"
			} else {
				startReq := lifecycle.StartRequest{
					ComposeFile: project,
					Services:    []string{instance},
				}
				if _, err := s.deps.Lifecycle.Start(r.Context(), startReq); err != nil {
					data.Error = err.Error()
					data.Status = "exited"
				}
			}
		}
	} else {
		data.Error = "Lifecycle service not available"
	}

	s.render.renderPartial(w, "instance", "content", data)
}

type composeEditData struct {
	Instance string
	FilePath string
	Content  string
	Error    string
	Saved    bool
}

func (s *Server) handleViewCompose(w http.ResponseWriter, r *http.Request) {
	instance := r.PathValue("instance")

	data := composeEditData{Instance: instance}

	composeFile, err := s.findProjectForService(instance)
	if err != nil {
		data.Error = err.Error()
		s.render.render(w, "compose-edit", data)
		return
	}
	data.FilePath = composeFile

	raw, err := os.ReadFile(composeFile)
	if err != nil {
		data.Error = fmt.Sprintf("read %s: %v", composeFile, err)
		s.render.render(w, "compose-edit", data)
		return
	}
	data.Content = string(raw)

	s.render.render(w, "compose-edit", data)
}

func (s *Server) handleEditCompose(w http.ResponseWriter, r *http.Request) {
	instance := r.PathValue("instance")

	data := composeEditData{Instance: instance}

	composeFile, err := s.findProjectForService(instance)
	if err != nil {
		data.Error = err.Error()
		s.render.renderPartial(w, "compose-edit", "content", data)
		return
	}
	data.FilePath = composeFile

	if err := r.ParseForm(); err != nil {
		data.Error = fmt.Sprintf("parse form: %v", err)
		s.render.renderPartial(w, "compose-edit", "content", data)
		return
	}
	content := r.FormValue("content")
	data.Content = content

	tmpFile, tmpErr := os.CreateTemp("", "ark-web-compose-validate-*.yaml")
	if tmpErr != nil {
		data.Error = fmt.Sprintf("create temp file: %v", tmpErr)
		s.render.renderPartial(w, "compose-edit", "content", data)
		return
	}
	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		data.Error = fmt.Sprintf("write temp file: %v", err)
		s.render.renderPartial(w, "compose-edit", "content", data)
		return
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	if _, err := s.deps.ComposeService.LoadProject(r.Context(), tmpFile.Name()); err != nil {
		data.Error = fmt.Sprintf("Invalid compose YAML: %v", err)
		s.render.renderPartial(w, "compose-edit", "content", data)
		return
	}

	if err := os.WriteFile(composeFile, []byte(content), 0644); err != nil {
		data.Error = fmt.Sprintf("write %s: %v", composeFile, err)
		s.render.renderPartial(w, "compose-edit", "content", data)
		return
	}

	data.Saved = true
	s.render.renderPartial(w, "compose-edit", "content", data)
}

func (s *Server) handleViewINI(w http.ResponseWriter, r *http.Request) {
	instance := r.PathValue("instance")
	s.logger.Info("view ini stub", "instance", instance)
	fmt.Fprintf(w, "// ini config for %s (stub)\n", instance)
}

func (s *Server) handleEditINI(w http.ResponseWriter, r *http.Request) {
	instance := r.PathValue("instance")
	s.logger.Info("edit ini stub", "instance", instance)
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (s *Server) findProjects() ([]string, error) {
	if s.deps != nil && s.deps.FindProjects != nil {
		return s.deps.FindProjects()
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("read working directory: %w", err)
	}
	var projects []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "docker-compose") &&
			(strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml")) {
			projects = append(projects, e.Name())
		}
	}
	return projects, nil
}

func (s *Server) findProjectForService(service string) (string, error) {
	projects, err := s.findProjects()
	if err != nil {
		return "", err
	}
	for _, p := range projects {
		proj, err := s.deps.ComposeService.LoadProject(context.Background(), p)
		if err != nil {
			continue
		}
		if err := proj.EnsureService(service); err == nil {
			return p, nil
		}
	}
	return "", ark.ErrNotFound
}

func (s *Server) ListenAndServe(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	srv := &http.Server{
		Handler: s.mux,
		Addr:    addr,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("web server starting", "addr", addr)
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case sig := <-sigCh:
		s.logger.Info("shutting down", "signal", sig)
		srv.Close()
		return nil
	case err := <-errCh:
		return err
	}
}
