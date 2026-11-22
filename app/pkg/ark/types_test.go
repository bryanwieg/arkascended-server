package ark

import (
	"reflect"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
)

func projectWith(services types.Services) *ComposeProject {
	return &ComposeProject{Project: &types.Project{Services: services}}
}

// TestOnlyLabelledServicesAreInstances verifies instances are the services with the ark.port.asa label.
// - Expect: the scheduler and an unlabelled service (such as ark-web) are not listed and EnsureService refuses them.
func TestOnlyLabelledServicesAreInstances(t *testing.T) {
	p := projectWith(types.Services{
		"asa2":           {Name: "asa2", Labels: types.Labels{PortASALabel: "7778"}},
		"asa1":           {Name: "asa1", Labels: types.Labels{PortASALabel: "7777"}},
		"ark-web":        {Name: "ark-web"},
		SchedulerService: {Name: SchedulerService, Labels: types.Labels{RoleLabel: SchedulerRole}},
	})
	if got := p.ServiceNames(); !reflect.DeepEqual(got, []string{"asa1", "asa2"}) {
		t.Fatalf("ServiceNames = %v", got)
	}
	if !p.HasScheduler() {
		t.Fatal("HasScheduler = false")
	}
	if err := p.EnsureService("asa1"); err != nil {
		t.Fatalf("EnsureService(asa1): %v", err)
	}
	err := p.EnsureService(SchedulerService)
	if err == nil || err.Error() != "Error: ark-scheduler is the scheduler service, not an ARK instance" {
		t.Fatalf("EnsureService(scheduler) = %v", err)
	}
	err = p.EnsureService("ark-web")
	if err == nil || err.Error() != "Error: ark-web is not an ARK instance (no ark.port.asa label)" {
		t.Fatalf("EnsureService(ark-web) = %v", err)
	}
	if err := p.EnsureService("nope"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("EnsureService(nope) = %v", err)
	}
}

// TestHasSchedulerFalseWithoutLabel verifies an unlabelled service named ark-scheduler is not the scheduler.
func TestHasSchedulerFalseWithoutLabel(t *testing.T) {
	p := projectWith(types.Services{SchedulerService: {Name: SchedulerService}})
	if p.HasScheduler() {
		t.Fatal("HasScheduler = true")
	}
}
