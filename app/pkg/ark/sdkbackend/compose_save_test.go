package sdkbackend

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"

	"pkg/ark"
	"pkg/ark/composeutil"
)

// saveRoundTrip writes files into a temp dir, loads compose.yaml, applies mutate, saves, and
// returns the saved text and SaveProject's error.
func saveRoundTrip(t *testing.T, files map[string]string, mutate func(*types.Project)) (string, error) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	composeFile := filepath.Join(dir, "compose.yaml")
	c, err := NewSDKCompose(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	project, err := c.LoadProject(context.Background(), composeFile)
	if err != nil {
		t.Fatal(err)
	}
	mutate(project.Project)
	saveErr := c.SaveProject(context.Background(), composeFile, project)
	saved, err := os.ReadFile(composeFile)
	if err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != len(files) {
		t.Fatalf("SaveProject left extra files: %v", entries)
	}
	return string(saved), saveErr
}

func mutateService(t *testing.T, p *types.Project, name string, fn func(*types.ServiceConfig)) {
	t.Helper()
	if err := composeutil.MutateService(p, name, fn); err != nil {
		t.Fatal(err)
	}
}

const richCompose = `# Cluster of two servers.
x-common: &common
  restart: unless-stopped  # shared
services:
  asa1:
    <<: *common
    image: ark-server:latest  # pinned by ark-cli
    env_file: secrets.env
    environment:
      - MAP_NAME=TheIsland_WP
      - "SERVER_ADMIN_PASSWORD=${ADMIN_PW}"   # from .env
    ports:
    - "7777:7777/udp"
    # RCON is local-only by default.
    - "127.0.0.1:27020:27020/tcp"
    labels:
      ark.port.asa: "7777"
      ark.port.rcon: "27020"
    volumes:
      - ./mods:/mods:ro

  spare:
    image: ark-server:latest
    profiles: ["spare"]
volumes:
  asa1-data: {}
`

func TestSaveProjectEditsOnlyChangedLines(t *testing.T) {
	files := map[string]string{"compose.yaml": richCompose, ".env": "ADMIN_PW=fromdotenv\n", "secrets.env": "STEAM_TOKEN=s3cret\n"}
	saved, err := saveRoundTrip(t, files, func(p *types.Project) {
		mutateService(t, p, "asa1", func(s *types.ServiceConfig) {
			s.Image = "ark-server:v2"
			composeutil.SetEnv(s.Environment, "SERVER_ADMIN_PASSWORD", "newAdmin1")
			composeutil.SetEnv(s.Environment, "SERVER_PASSWORD", "join$1")
			s.Ports[1].HostIP = ""
			s.Ports[1].Published = "27021"
			s.Labels["ark.port.rcon"] = "27021"
			s.Ports = append(s.Ports, types.ServicePortConfig{Target: 8080, Published: "8080", Protocol: "tcp"})
		})
	})
	if err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	want := strings.NewReplacer(
		"    image: ark-server:latest  # pinned by ark-cli\n", "    image: ark-server:v2\n",
		`      - "SERVER_ADMIN_PASSWORD=${ADMIN_PW}"   # from .env`+"\n",
		"      - SERVER_ADMIN_PASSWORD=newAdmin1\n      - \"SERVER_PASSWORD=join$$1\"\n",
		`    - "127.0.0.1:27020:27020/tcp"`+"\n", `    - "27021:27020/tcp"`+"\n"+`    - "8080:8080/tcp"`+"\n",
		`      ark.port.rcon: "27020"`, `      ark.port.rcon: "27021"`,
	).Replace(richCompose)
	if saved != want {
		t.Fatalf("saved file differs.\n--- got\n%s\n--- want\n%s", saved, want)
	}
}

func TestSaveProjectMapFormAndMissingSections(t *testing.T) {
	compose := "services:\n  asa1:\n    image: ark-server:latest\n    environment:\n      MAP_NAME: TheIsland_WP\n  asa2:\n    image: ark-server:latest\n"
	saved, err := saveRoundTrip(t, map[string]string{"compose.yaml": compose}, func(p *types.Project) {
		mutateService(t, p, "asa1", func(s *types.ServiceConfig) {
			composeutil.SetEnv(s.Environment, "MAP_NAME", "Ragnarok_WP")
			composeutil.SetEnv(s.Environment, "ASA_PORT", "7778")
		})
		mutateService(t, p, "asa2", func(s *types.ServiceConfig) {
			s.Environment = types.MappingWithEquals{}
			composeutil.SetEnv(s.Environment, "ASA_PORT", "7779")
			s.Ports = []types.ServicePortConfig{{Target: 7779, Published: "7779", Protocol: "udp"}}
		})
	})
	if err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	want := "services:\n  asa1:\n    image: ark-server:latest\n    environment:\n      MAP_NAME: Ragnarok_WP\n      ASA_PORT: \"7778\"\n" +
		"  asa2:\n    image: ark-server:latest\n    environment:\n      - ASA_PORT=7779\n    ports:\n      - \"7779:7779/udp\"\n"
	if saved != want {
		t.Fatalf("saved file differs.\n--- got\n%s\n--- want\n%s", saved, want)
	}
}

func TestSaveProjectSetsCPUShares(t *testing.T) {
	compose := "services:\n  asa1:\n    image: ark-server:latest\n    mem_reservation: 12G\n  asa2:\n    image: ark-server:latest\n    cpu_shares: 1024  # old\n    init: true\n"
	saved, err := saveRoundTrip(t, map[string]string{"compose.yaml": compose}, func(p *types.Project) {
		mutateService(t, p, "asa1", func(s *types.ServiceConfig) { s.CPUShares = 4096 })
		mutateService(t, p, "asa2", func(s *types.ServiceConfig) { s.CPUShares = 8192 })
	})
	if err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	want := "services:\n  asa1:\n    image: ark-server:latest\n    mem_reservation: 12G\n    cpu_shares: 4096\n" +
		"  asa2:\n    image: ark-server:latest\n    cpu_shares: 8192\n    init: true\n"
	if saved != want {
		t.Fatalf("saved file differs.\n--- got\n%s\n--- want\n%s", saved, want)
	}
}

func TestSaveProjectRefusesEditsItCannotMakeSafely(t *testing.T) {
	tests := []struct {
		name    string
		compose string
		mutate  func(*types.ServiceConfig)
	}{
		{
			name:    "flow-style labels",
			compose: "services:\n  asa1:\n    image: ark-server:latest\n    labels: { ark.port.asa: \"7777\" }\n",
			mutate:  func(s *types.ServiceConfig) { s.Labels["ark.port.asa"] = "7778" },
		},
		{
			// Adding environment: to the service would replace the merged list, losing MAP_NAME.
			name:    "environment inherited through a merge key",
			compose: "x-base: &base\n  environment:\n    - MAP_NAME=TheIsland_WP\nservices:\n  asa1:\n    <<: *base\n    image: ark-server:latest\n",
			mutate:  func(s *types.ServiceConfig) { composeutil.SetEnv(s.Environment, "ASA_PORT", "7778") },
		},
		{
			name:    "cpu_shares removed",
			compose: "services:\n  asa1:\n    image: ark-server:latest\n    cpu_shares: 4096\n",
			mutate:  func(s *types.ServiceConfig) { s.CPUShares = 0 },
		},
		{
			name:    "change outside the editable fields",
			compose: "services:\n  asa1:\n    image: ark-server:latest\n",
			mutate:  func(s *types.ServiceConfig) { s.ContainerName = "renamed" },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saved, err := saveRoundTrip(t, map[string]string{"compose.yaml": tt.compose}, func(p *types.Project) {
				mutateService(t, p, "asa1", tt.mutate)
			})
			if err == nil {
				t.Fatalf("expected an error, file became:\n%s", saved)
			}
			t.Logf("refused: %v", err)
			if saved != tt.compose {
				t.Fatalf("file must stay unchanged on error, got:\n%s", saved)
			}
		})
	}
}

func TestSaveProjectKeepsWhatTheOldSaveLost(t *testing.T) {
	files := map[string]string{"compose.yaml": richCompose, ".env": "ADMIN_PW=fromdotenv\n", "secrets.env": "STEAM_TOKEN=s3cret\n"}
	saved, err := saveRoundTrip(t, files, func(p *types.Project) {
		mutateService(t, p, "asa1", func(s *types.ServiceConfig) { s.Image = "ark-server:v2" })
	})
	if err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	checks := []struct{ what, text string }{
		{"the ${VAR} reference", "${ADMIN_PW}"},
		{"the env_file reference", "env_file: secrets.env"},
		{"the relative bind path", "./mods:/mods:ro"},
		{"the inactive-profile service", "  spare:\n"},
		{"the anchor and merge key", "<<: *common"},
		{"comments", "# RCON is local-only by default."},
		{"short port syntax", `- "7777:7777/udp"`},
	}
	for _, c := range checks {
		if !strings.Contains(saved, c.text) {
			t.Errorf("save lost %s (%q):\n%s", c.what, c.text, saved)
		}
	}
	for _, leaked := range []string{"fromdotenv", "s3cret", "name:", "networks:"} {
		if strings.Contains(saved, leaked) {
			t.Errorf("save wrote loader-resolved content %q into the file:\n%s", leaked, saved)
		}
	}
	if got, want := len(strings.Split(saved, "\n")), len(strings.Split(richCompose, "\n")); got != want {
		t.Errorf("an image change should rewrite one line, line count went %d -> %d", want, got)
	}
}

func TestSaveProjectWithoutChangesLeavesFileAlone(t *testing.T) {
	saved, err := saveRoundTrip(t, map[string]string{"compose.yaml": richCompose, ".env": "ADMIN_PW=x\n", "secrets.env": ""}, func(*types.Project) {})
	if err != nil || saved != richCompose {
		t.Fatalf("expected an untouched file and no error, got err=%v:\n%s", err, saved)
	}
}

// Values must load back exactly as set: YAML-special characters are quoted and $ is escaped
// so compose does not interpolate it.
func TestSaveProjectValuesLoadBackUnchanged(t *testing.T) {
	values := []string{"My Server #1", "a: b", "-dash", "true", "7777", "pa$$word", "${NOT_A_VAR}", "quote\"and'apostrophe", "", "Ünïcode"}
	for _, form := range []string{
		"services:\n  asa1:\n    image: ark-server:latest\n    environment:\n      - SESSION_NAME=old\n",
		"services:\n  asa1:\n    image: ark-server:latest\n    environment:\n      SESSION_NAME: old\n",
	} {
		for _, v := range values {
			dir := t.TempDir()
			composeFile := filepath.Join(dir, "compose.yaml")
			if err := os.WriteFile(composeFile, []byte(form), 0o644); err != nil {
				t.Fatal(err)
			}
			c, err := NewSDKCompose(slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err != nil {
				t.Fatal(err)
			}
			project, err := c.LoadProject(context.Background(), composeFile)
			if err != nil {
				t.Fatal(err)
			}
			mutateService(t, project.Project, "asa1", func(s *types.ServiceConfig) {
				composeutil.SetEnv(s.Environment, "SESSION_NAME", v)
				s.Labels = types.Labels{"note": v}
			})
			if err := c.SaveProject(context.Background(), composeFile, project); err != nil {
				t.Fatalf("value %q: SaveProject: %v", v, err)
			}
			reloaded, err := c.LoadProject(context.Background(), composeFile)
			if err != nil {
				t.Fatalf("value %q: reload: %v", v, err)
			}
			svc := reloaded.Project.Services["asa1"]
			if got, _ := composeutil.EnvValue(svc.Environment, "SESSION_NAME"); got != v {
				t.Errorf("env value %q loaded back as %q", v, got)
			}
			if got := svc.Labels["note"]; got != v {
				t.Errorf("label value %q loaded back as %q", v, got)
			}
		}
	}
}

func TestSaveProjectRefusesPortRanges(t *testing.T) {
	compose := "services:\n  asa1:\n    image: ark-server:latest\n    ports:\n      - \"7777-7778:7777-7778/udp\"\n"
	saved, err := saveRoundTrip(t, map[string]string{"compose.yaml": compose}, func(p *types.Project) {
		mutateService(t, p, "asa1", func(s *types.ServiceConfig) { s.Ports[0].Published = "8888" })
	})
	if err == nil || saved != compose {
		t.Fatalf("expected a refusal and an unchanged file, got err=%v:\n%s", err, saved)
	}
}

func newTestCompose(t *testing.T) *SDKCompose {
	t.Helper()
	c, err := NewSDKCompose(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func schedule(name, cron string) map[string]any {
	return map[string]any{"cron": cron, "enabled": true, "instance": "asa1", "name": name, "type": "backup"}
}

const schedulesCompose = `services:
  asa1:
    image: ark-server:latest
# Managed by ark-cli.
x-ark-schedules:
  - cron: "0 3 * * *"
    enabled: true
    instance: asa1
    name: nightly
    type: backup
# Kept below the block.
volumes:
  data: {}
`

const nightlyBlock = `x-ark-schedules:
  - cron: 0 3 * * *
    enabled: true
    instance: asa1
    name: nightly
    type: backup
`

func TestSaveProjectWritesSchedulesExtension(t *testing.T) {
	setSchedules := func(v any) func(*types.Project) {
		return func(p *types.Project) { p.Extensions = types.Extensions{ark.SchedulesExtensionKey: v} }
	}
	tests := []struct {
		name, compose, want string
		mutate              func(*types.Project)
	}{
		{
			name:    "append keeps every earlier line",
			compose: richCompose,
			mutate: func(p *types.Project) {
				p.Extensions[ark.SchedulesExtensionKey] = []any{schedule("nightly", "0 3 * * *")}
			},
			want: richCompose + nightlyBlock,
		},
		{
			name:    "replace keeps comments around the block",
			compose: schedulesCompose,
			mutate:  setSchedules([]any{schedule("nightly", "0 4 * * *")}),
			want:    strings.Replace(schedulesCompose, `"0 3 * * *"`, "0 4 * * *", 1),
		},
		{
			name:    "remove",
			compose: schedulesCompose,
			mutate:  func(p *types.Project) { delete(p.Extensions, ark.SchedulesExtensionKey) },
			want:    "services:\n  asa1:\n    image: ark-server:latest\n# Managed by ark-cli.\n# Kept below the block.\nvolumes:\n  data: {}\n",
		},
		{
			name:    "flow-style empty services",
			compose: "services: {}\n",
			mutate:  setSchedules([]any{schedule("nightly", "0 3 * * *")}),
			want:    "services: {}\n" + nightlyBlock,
		},
		{
			name:    "dollar signs are escaped",
			compose: "services: {}\n",
			mutate:  setSchedules([]any{schedule("pay $5", "${X}")}),
			want:    "services: {}\nx-ark-schedules:\n  - cron: $${X}\n    enabled: true\n    instance: asa1\n    name: pay $$5\n    type: backup\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saved, err := saveRoundTrip(t, map[string]string{"compose.yaml": tt.compose, "secrets.env": "STEAM_TOKEN=s3cret\n"}, tt.mutate)
			if err != nil {
				t.Fatalf("SaveProject: %v", err)
			}
			if saved != tt.want {
				t.Fatalf("saved file differs.\n--- got\n%s\n--- want\n%s", saved, tt.want)
			}
		})
	}
	t.Run("dollar values load back uninterpolated", func(t *testing.T) {
		want := []any{schedule("pay $5", "${X}")}
		saved, err := saveRoundTrip(t, map[string]string{"compose.yaml": "services: {}\n"}, setSchedules(want))
		if err != nil {
			t.Fatal(err)
		}
		composeFile := filepath.Join(t.TempDir(), "compose.yaml")
		if err := os.WriteFile(composeFile, []byte(saved), 0o644); err != nil {
			t.Fatal(err)
		}
		loaded, err := newTestCompose(t).LoadProject(context.Background(), composeFile)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(loaded.LoadedSchedules, want) {
			t.Fatalf("loaded %#v, want %#v", loaded.LoadedSchedules, want)
		}
	})
}

func TestSaveProjectRefusesOtherExtensionChanges(t *testing.T) {
	tests := map[string]func(*types.Project){
		"other extension":    func(p *types.Project) { p.Extensions["x-common"] = map[string]any{"restart": "always"} },
		"non-list schedules": func(p *types.Project) { p.Extensions[ark.SchedulesExtensionKey] = "nightly" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			saved, err := saveRoundTrip(t, map[string]string{"compose.yaml": richCompose, "secrets.env": "STEAM_TOKEN=s3cret\n"}, mutate)
			if !errors.Is(err, errUnsupportedSave) || saved != richCompose {
				t.Fatalf("expected errUnsupportedSave and an unchanged file, got err=%v:\n%s", err, saved)
			}
		})
	}
}

func TestSaveProjectKeepsSchedulesItDidNotChange(t *testing.T) {
	ctx := context.Background()
	composeFile := filepath.Join(t.TempDir(), "compose.yaml")
	if err := os.WriteFile(composeFile, []byte(schedulesCompose), 0o644); err != nil {
		t.Fatal(err)
	}
	c := newTestCompose(t)
	load := func() *ark.ComposeProject {
		p, err := c.LoadProject(ctx, composeFile)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	stale, other := load(), load()
	both := []any{schedule("nightly", "0 3 * * *"), schedule("weekly", "0 5 * * 0")}
	other.Project.Extensions[ark.SchedulesExtensionKey] = both
	if err := c.SaveProject(ctx, composeFile, other); err != nil {
		t.Fatal(err)
	}
	mutateService(t, stale.Project, "asa1", func(s *types.ServiceConfig) { s.Image = "ark-server:v2" })
	if err := c.SaveProject(ctx, composeFile, stale); err != nil {
		t.Fatal(err)
	}
	reloaded := load()
	if !reflect.DeepEqual(reloaded.LoadedSchedules, both) {
		t.Fatalf("schedules = %#v, want %#v", reloaded.LoadedSchedules, both)
	}
	if got := reloaded.Project.Services["asa1"].Image; got != "ark-server:v2" {
		t.Fatalf("image = %q, want ark-server:v2", got)
	}
}
