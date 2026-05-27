package main

import (
	"context"
	"fmt"
)

// ControlPlane abstracts the server management layer. It determines WHAT
// commands to run (kubectl, docker, local shell) while the Executor determines
// WHERE they run (locally or over SSH).
type ControlPlane interface {
	// Name returns the control plane identifier for status reporting.
	Name() string

	// GetStatus returns the battlegroup status and per-server runtime stats.
	GetStatus(ctx context.Context, exec Executor) (*BattlegroupStatus, error)

	// ExecCommand runs a lifecycle command: start, stop, restart, update, backup.
	ExecCommand(ctx context.Context, exec Executor, cmd string) (string, error)

	// ListProcesses returns running processes/pods/containers and a context label.
	ListProcesses(ctx context.Context, exec Executor) ([]ProcessInfo, string, error)

	// ListLogSources returns available log sources (pods, containers, services).
	ListLogSources(ctx context.Context, exec Executor) ([]LogSource, error)

	// StreamLog opens a log stream for the named source. The caller must invoke
	// cancel when done to release the underlying session/process.
	StreamLog(ctx context.Context, exec Executor, ns, name string) (<-chan string, func(), error)

	// CaptureJWT extracts the ServiceAuthToken from the game daemon and returns
	// a HostId and freshly-signed JWT for broker authentication.
	CaptureJWT(ctx context.Context, exec Executor) (hostID, token string, err error)

	// EvalOnGameBroker runs an Erlang expression via rabbitmqctl eval inside the
	// mq-game broker. Used for publishing server commands with user_id="fls",
	// which AMQP connections cannot set (broker validates UserId against auth'd user).
	EvalOnGameBroker(ctx context.Context, exec Executor, expr string) (string, error)

	// DiscoverIniDir returns the directory containing UserGame.ini and
	// UserOverrides.ini. kubectl auto-discovers this from k3s storage;
	// docker and local require server_ini_dir to be set in config.
	DiscoverIniDir(ctx context.Context, exec Executor) (string, error)

	// ReadDefaultINI reads DefaultGame.ini or DefaultEngine.ini from inside the
	// game container/pod, where the file lives as part of the image. Returns the
	// file contents or "" if unavailable. The local control plane returns "" and
	// lets the host-path fallback handle it.
	ReadDefaultINI(ctx context.Context, exec Executor, filename string) string
}

// ── Types shared across control plane implementations ─────────────────────────

type BattlegroupStatus struct {
	Name     string      `json:"name"`
	Title    string      `json:"title"`
	Phase    string      `json:"phase"`
	Database string      `json:"database"`
	Servers  []ServerRow `json:"servers"`
}

type ServerRow struct {
	Map       string `json:"map"`
	Sietch    string `json:"sietch"`
	Dimension int    `json:"dimension"`
	Partition int    `json:"partition"`
	Phase     string `json:"phase"`
	Ready     bool   `json:"ready"`
	Players   int    `json:"players"`
}

type ProcessInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status,omitempty"`
}

type LogSource struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// ── Factory ───────────────────────────────────────────────────────────────────

// newControlPlane returns the appropriate ControlPlane based on the control
// name ("kubectl", "docker", "local", "amp"). Unrecognised names fall back to local.
func newControlPlane(name string, cfg appConfig) ControlPlane {
	switch name {
	case "kubectl":
		return &kubectlControl{namespace: cfg.ControlNamespace}
	case "docker":
		return &dockerControl{
			gameserver:  cfg.DockerGameserver,
			brokerGame:  cfg.DockerBrokerGame,
			brokerAdmin: cfg.DockerBrokerAdmin,
		}
	case "amp":
		user := cfg.AmpUser
		if user == "" {
			user = "amp"
		}
		container := cfg.AmpContainer
		if container == "" && cfg.AmpInstance != "" {
			container = "AMP_" + cfg.AmpInstance
		}
		// Default to container mode (CubeCoders' standard template) unless the
		// admin explicitly opts out.
		useContainer := true
		if cfg.AmpUseContainer != nil {
			useContainer = *cfg.AmpUseContainer
		}
		return &ampControl{
			instance:     cfg.AmpInstance,
			container:    container,
			ampUser:      user,
			logPath:      cfg.AmpLogPath,
			directorURL:  cfg.DirectorURL,
			iniDir:       cfg.ServerIniDir,
			useContainer: useContainer,
			dataRoot:     cfg.AmpDataRoot,
		}
	default:
		return &localControl{
			cmdStart:         cfg.CmdStart,
			cmdStop:          cfg.CmdStop,
			cmdRestart:       cfg.CmdRestart,
			cmdStatus:        cfg.CmdStatus,
			brokerExecPrefix: cfg.BrokerExecPrefix,
		}
	}
}

// errNotSupported returns a consistent "not supported" error for control plane
// methods that are not available in a given implementation.
func errNotSupported(control, method string) error {
	return fmt.Errorf("%s control plane does not support %s", control, method)
}
