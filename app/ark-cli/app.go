package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"pkg/ark/host"
	"pkg/ark/runtime"
	"pkg/ark/sdkbackend"

	"ark-cli/internal/cmd"
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
)

type CLI struct {
	deps *core.AppDeps
}

func NewCLI(deps *core.AppDeps) *CLI {
	return &CLI{deps: deps}
}

func (c *CLI) Run(args []string) error {
	var runErr error
	core.WithAppDeps(c.deps, func() {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		core.CurrentAppDeps().Ctx = ctx

		opts, remaining, err := parseGlobalOptions(args)
		if err != nil {
			runErr = err
			return
		}
		if err := configureVerbose(opts); err != nil {
			runErr = fmt.Errorf("Error: failed to configure verbose logging: %v", err)
			return
		}
		defer closeVerbose()
		core.CurrentAppDeps().Logger = core.NewCLILogger(opts.verbose)
		if opts.help {
			usage()
			return
		}
		if opts.version {
			infoln(version)
			return
		}

		rt, rtErr := runtime.ResolveContainerRuntime(opts.runtime, core.CurrentAppDeps().Runtime.LookPath)
		if rtErr != nil {
			runErr = rtErr
			return
		}
		core.CurrentAppDeps().Runtime.ContainerRuntime = rt
		verboseLogf("[runtime] container runtime: %s\n", rt)

		// Create SDK backends for the resolved container runtime.
		// Validate socket availability and daemon version before SDK init.
		if rt == runtime.RuntimeDocker {
			dockerHost, ok := host.HasDockerService()
			if !ok {
				runErr = fmt.Errorf("Error: Docker socket not found at /var/run/docker.sock and DOCKER_HOST is not set. Is Docker installed and running?")
				return
			}
			verboseLogf("[runtime] Docker host: %s\n", dockerHost)
		}
		if rt == runtime.RuntimePodman {
			socketURI, rootfulDetected := host.HasPodmanService()
			if rootfulDetected {
				runErr = fmt.Errorf("Error: rootful podman socket detected at /run/podman/podman.sock, but only rootless podman is supported. Install and run podman as a non-root user (see https://podman.io/getting-started/installation#linux)")
				return
			}
			if socketURI == "" {
				runErr = fmt.Errorf("Error: podman system service not available. Run 'systemctl --user start podman.socket' or 'podman --log-level=debug system service -t0' (rootless only)")
				return
			}
			// Set DOCKER_HOST so the Docker SDK client connects to podman.
			if err := os.Setenv("DOCKER_HOST", socketURI); err != nil {
				runErr = fmt.Errorf("Error: failed to set DOCKER_HOST: %v", err)
				return
			}
			verboseLogf("[runtime] SDK backend using podman socket: %s\n", socketURI)
		}

		sdkEng, sdkErr := sdkbackend.NewSDKEngine(core.CurrentAppDeps().Logger)
		if sdkErr != nil {
			runErr = fmt.Errorf("Error: failed to initialize SDK backend: %v", sdkErr)
			return
		}
		sdkComp, sdkErr := sdkbackend.NewSDKCompose(core.CurrentAppDeps().Logger)
		if sdkErr != nil {
			runErr = fmt.Errorf("Error: failed to initialize SDK Compose backend: %v", sdkErr)
			return
		}
		core.CurrentAppDeps().Engine = sdkEng
		core.CurrentAppDeps().ComposeService = sdkComp
		verboseLogf("[runtime] backend: sdk\n")

		// Validate daemon version via SDK.
		if err := sdkEng.ValidateDaemonVersion(core.CurrentAppDeps().Ctx, string(rt)); err != nil {
			runErr = err
			return
		}

		// Wire up RuntimeHelper with resolved runtime and services.
		d := core.CurrentAppDeps()
		core.FillDefaults(d)

		// Validate compose provider availability.
		if rtErr := d.RuntimeHelper.ValidateComposeProvider(); rtErr != nil {
			runErr = rtErr
			return
		}

		if len(remaining) < 1 {
			usage()
			return
		}
		command, commandArgs, err := parseCommandRoute(remaining)
		if err != nil {
			runErr = err
			return
		}
		handler, ok := cmd.CommandRegistryRouteHandler(command)
		if !ok {
			usage()
			return
		}
		runErr = handler(commandArgs)
	})
	return runErr
}

func (c *CLI) Main(args []string) {
	if err := c.Run(args); err != nil {
		core.Die(err.Error())
	}
}

func Main() {
	deps := core.DefaultAppDeps()
	compose.WireDefaults(deps)
	cmd.WireDefaults(deps)
	core.SetCurrentAppDeps(deps)

	NewCLI(deps).Main(os.Args[1:])
}
