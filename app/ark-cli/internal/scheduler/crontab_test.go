package scheduler

// Tests: StripManagedCrontab keeps only user lines; RemoveManagedCrontab runs the right crontab commands

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"ark-cli/internal/core"
	"ark-cli/internal/testutil"

	"pkg/ark"
)

const managedBlock = "# ark-cli managed schedules\n" +
	"# Do not edit manually; regenerate via ark-cli schedule commands.\n" +
	"# schedule nightly: ark-cli create backup --instance island\n" +
	"0 3 * * * cd '/srv/ark' && 'ark-cli' 'create' 'backup' '--instance' 'island' >> '/srv/ark/.ark-manage/logs/schedules/nightly.log' 2>&1\n" +
	"# schedule hourly: ark-cli create backup --instance center\n" +
	"0 * * * * cd '/srv/ark' && 'ark-cli' 'create' 'backup' '--instance' 'center' >> '/srv/ark/.ark-manage/logs/schedules/hourly.log' 2>&1\n"

func TestStripManagedCrontab(t *testing.T) {
	cases := []struct {
		name, in, want string
		changed        bool
	}{
		{"only managed", managedBlock, "", true},
		{
			"user lines before and after",
			"MAILTO=me\n@reboot /usr/bin/true\n" + managedBlock + "# my job\n5 4 * * * /home/me/job.sh\n",
			"MAILTO=me\n@reboot /usr/bin/true\n# my job\n5 4 * * * /home/me/job.sh\n",
			true,
		},
		{
			"no header",
			"# schedule mine: keep\n5 4 * * * /home/me/job.sh\n",
			"# schedule mine: keep\n5 4 * * * /home/me/job.sh\n",
			false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := StripManagedCrontab(tc.in)
			if got != tc.want || changed != tc.changed {
				t.Fatalf("got (%q, %v), want (%q, %v)", got, changed, tc.want, tc.changed)
			}
		})
	}
}

// crontabDeps fakes `crontab`: listing returns list (exit listExit); every call is recorded,
// with the installed file's content captured at call time.
func crontabDeps(haveBinary bool, list string, listExit int) (*core.AppDeps, *[][]string, *string) {
	var calls [][]string
	var installed string
	d := &core.AppDeps{Runtime: core.RuntimeDeps{
		LookPath: func(string) (string, error) {
			if !haveBinary {
				return "", errors.New("not found")
			}
			return "/usr/bin/crontab", nil
		},
		RunCommand: func(cmd []string, _ ark.CommandOptions) (ark.ExecResult, error) {
			calls = append(calls, cmd)
			if len(cmd) == 2 && cmd[1] == "-l" {
				return ark.ExecResult{Stdout: list, ExitCode: listExit}, nil
			}
			if len(cmd) == 2 && cmd[1] != "-r" {
				data, err := os.ReadFile(cmd[1])
				if err != nil {
					return ark.ExecResult{}, err
				}
				installed = string(data)
			}
			return ark.ExecResult{}, nil
		},
	}}
	return d, &calls, &installed
}

func TestRemoveManagedCrontab(t *testing.T) {
	testutil.SetEnv(t, "ARK_TMP_DIR", t.TempDir())
	user := "MAILTO=me\n5 4 * * * /home/me/job.sh\n"

	t.Run("no binary", func(t *testing.T) {
		d, calls, _ := crontabDeps(false, "", 0)
		if err := RemoveManagedCrontab(d); err != nil || len(*calls) != 0 {
			t.Fatalf("err=%v calls=%v", err, *calls)
		}
	})
	t.Run("no crontab", func(t *testing.T) {
		d, calls, _ := crontabDeps(true, "", 1)
		if err := RemoveManagedCrontab(d); err != nil || len(*calls) != 1 {
			t.Fatalf("err=%v calls=%v", err, *calls)
		}
	})
	t.Run("no managed block", func(t *testing.T) {
		d, calls, _ := crontabDeps(true, user, 0)
		out := testutil.CaptureStdout(t, func() {
			if err := RemoveManagedCrontab(d); err != nil {
				t.Fatal(err)
			}
		})
		if len(*calls) != 1 || out != "" {
			t.Fatalf("calls=%v out=%q", *calls, out)
		}
	})
	t.Run("replace", func(t *testing.T) {
		d, calls, installed := crontabDeps(true, managedBlock+user, 0)
		out := testutil.CaptureStdout(t, func() {
			if err := RemoveManagedCrontab(d); err != nil {
				t.Fatal(err)
			}
		})
		if len(*calls) != 2 || (*calls)[1][0] != "crontab" || *installed != user {
			t.Fatalf("calls=%v installed=%q", *calls, *installed)
		}
		if _, err := os.Stat((*calls)[1][1]); !os.IsNotExist(err) {
			t.Fatalf("temp crontab not removed: %v", err)
		}
		if !strings.Contains(out, "Removed ark-cli schedules from your crontab") {
			t.Fatalf("out=%q", out)
		}
	})
	t.Run("remove", func(t *testing.T) {
		d, calls, _ := crontabDeps(true, managedBlock+"\n", 0)
		out := testutil.CaptureStdout(t, func() {
			if err := RemoveManagedCrontab(d); err != nil {
				t.Fatal(err)
			}
		})
		want := [][]string{{"crontab", "-l"}, {"crontab", "-r"}}
		if !reflect.DeepEqual(*calls, want) || !strings.Contains(out, "Removed ark-cli schedules") {
			t.Fatalf("calls=%v out=%q", *calls, out)
		}
	})
}
