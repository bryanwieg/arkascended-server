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

// TestSchedulerServiceIsNotAnInstance verifies the labelled scheduler is hidden from instance enumeration.
func TestSchedulerServiceIsNotAnInstance(t *testing.T) {
	p := projectWith(types.Services{
		"asa2":           {Name: "asa2"},
		"asa1":           {Name: "asa1", Labels: types.Labels{RoleLabel: "other"}},
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
	if got := p.ServiceNames(); !reflect.DeepEqual(got, []string{SchedulerService}) {
		t.Fatalf("ServiceNames = %v", got)
	}
}
