package cli

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/apourchet/kodder/lib/client"
	"github.com/uber/makisu/lib/log"
)

// ClientApplication is the struct that can talk to the Kodder worker through a socket or HTTP.
type ClientApplication struct {
	LocalSharedPath  string  `commander:"flag=local,The absolute path of the local mountpoint shared with the makisu worker."`
	WorkerSharedPath string  `commander:"flag=shared,The absolute destination of the mountpoint shared with the makisu worker."`
	HTTPAddress      string  `commander:"flag=address,The address of the Kodder worker."`
	Socket           *string `commander:"flag=socket,The absolute path of the unix socket that the makisu worker listens on."`

	WaitDuration time.Duration `commander:"flag=wait,The time to wait for worker to ready up (valid for build and abort calls). Format follows the golang time.Duration spec."`
}

var defaultDockerfilePath = "Dockerfile"

// NewClientApplication returns a new ClientApplication to talk to the Kodder worker.
func NewClientApplication() *ClientApplication {
	return &ClientApplication{
		LocalSharedPath:  defaultEnv("KODDER_MNT_LOCAL", "/kodder"),
		WorkerSharedPath: defaultEnv("KODDER_MNT_REMOTE", "/kodder"),
		HTTPAddress:      defaultEnv("KODDERD_ADDR", "localhost:3456"),

		WaitDuration: 10 * time.Second,
	}
}

// HandleSignals will read through the signals sent by the OS and
// abort the build if the signal is terminal.
func (app *ClientApplication) HandleSignals() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	for sig := range c {
		log.Infof("Got signal %v", sig)
		if sig == syscall.SIGTERM || sig == syscall.SIGKILL || sig == syscall.SIGINT ||
			sig == syscall.SIGSTOP || sig == syscall.SIGHUP {
			log.Infof("Signal was terminal; exiting")
			go func() {
				if err := app.Abort(); err == nil {
					os.Exit(1)
				}
			}()
		}
	}
}

// CommanderDefault gets called when no subcommand is specified.
func (app *ClientApplication) CommanderDefault() error {
	return fmt.Errorf("command needed; one of `ready`, `build`, `abort`")
}

// Ready returns an error if the worker is not ready to accept builds.
func (app *ClientApplication) Ready() error {
	if ready, err := app.client().Ready(); err != nil {
		return err
	} else if !ready {
		return fmt.Errorf("worker not ready")
	}
	log.Infof("Worker is ready")
	return nil
}

// Abort aborts the current build.
func (app *ClientApplication) Abort() error {
	if err := app.client().Abort(); err != nil {
		return err
	} else if err := app.waitReady(); err != nil {
		return err
	}
	log.Infof("Worker build aborted")
	return nil
}

// Build starts a build on the worker after copying the context over to it.
func (app *ClientApplication) Build(context string, makisuArgs []string) error {
	target, err := prepContext(app.LocalSharedPath, app.WorkerSharedPath, context)
	if err != nil {
		return fmt.Errorf("failed to prepare context: %v", err)
	}
	localPath := filepath.Join(app.LocalSharedPath, target)
	defer os.RemoveAll(localPath)
	workerPath := filepath.Join(app.WorkerSharedPath, target)

	start := time.Now()
	for time.Since(start) < app.WaitDuration {
		if err = app.client().Build(makisuArgs, workerPath); err == client.ErrWorkerBusy {
			time.Sleep(250 * time.Millisecond)
			continue
		} else if err != nil {
			return fmt.Errorf("build failed: %v", err)
		}
		return nil
	}
	return err
}

func (app *ClientApplication) waitReady() error {
	var err error
	var ready bool
	start := time.Now()
	for time.Since(start) < app.WaitDuration {
		if ready, err = app.client().Ready(); err == nil && ready {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return err
}

func (app *ClientApplication) client() *client.KodderClient {
	if app.Socket == nil {
		return client.NewWithAddress(app.HTTPAddress)
	}
	return client.NewWithSocket(*app.Socket)
}
