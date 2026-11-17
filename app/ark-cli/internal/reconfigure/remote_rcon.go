package reconfigure

import (
	"errors"

	"ark-cli/internal/core"

	"pkg/ark/runtime"
)

// RunEnableRemoteRCON publishes an instance's RCON port on all host interfaces.
func RunEnableRemoteRCON(_ *core.AppDeps, args []string) error {
	return runRemoteRCON(args, "enable remote-rcon", "Publish RCON on all host interfaces so remote RCON tools can connect.", "")
}

// RunDisableRemoteRCON publishes an instance's RCON port on 127.0.0.1 only (the default).
func RunDisableRemoteRCON(_ *core.AppDeps, args []string) error {
	return runRemoteRCON(args, "disable remote-rcon", "Publish RCON on 127.0.0.1 only, so it is reachable only from the Docker host.", rconLocalHostIP)
}

func runRemoteRCON(args []string, name, description, hostIP string) error {
	fs := core.NewCommandFlagSet(name, []string{name + " --instance <instance> [--force] [--restart]"}, description)
	instanceFlag := fs.String("instance", "", "Instance name.")
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")
	force := fs.Bool("force", false, "Allow the change while the instance is running (restart required for it to take effect).")
	restartNow := fs.Bool("restart", false, "Restart the target instance immediately after updating compose.")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	if *instanceFlag == "" {
		return errors.New("Error: --instance is required.")
	}
	if err := runtime.ValidateComposeName(*instanceFlag, "instance"); err != nil {
		return err
	}
	if hostIP == "" {
		core.Warnf("RCON sends the admin password in plain text. Docker-published ports bypass ufw/firewalld; " +
			"restrict who can reach RCON with a DOCKER-USER iptables rule.\n")
	}
	return runReconfigureCommand(reconfigureCommandRequest{
		instance:        *instanceFlag,
		composeFileFlag: *composeFileFlag,
		force:           *force,
		restartNow:      *restartNow,
		rconHostIPSet:   true,
		rconHostIP:      hostIP,
	})
}
