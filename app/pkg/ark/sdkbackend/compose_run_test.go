package sdkbackend

import (
	"slices"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
)

func TestSelectRunServiceExcludesOtherInstances(t *testing.T) {
	project := &types.Project{Services: types.Services{
		"asa1": {Name: "asa1", DependsOn: types.DependsOnConfig{"db": {Condition: types.ServiceConditionStarted}}},
		"asa2": {Name: "asa2"},
		"db":   {Name: "db"},
	}}

	for _, tc := range []struct {
		noDeps bool
		want   []string
	}{
		{noDeps: false, want: []string{"asa1", "db"}},
		{noDeps: true, want: []string{"asa1"}},
	} {
		got, err := selectRunService(project, "asa1", tc.noDeps)
		if err != nil {
			t.Fatalf("noDeps=%v: %v", tc.noDeps, err)
		}
		names := got.ServiceNames()
		slices.Sort(names)
		if !slices.Equal(names, tc.want) {
			t.Errorf("noDeps=%v: services = %v, want %v", tc.noDeps, names, tc.want)
		}
	}
}
