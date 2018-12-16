package cli

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/apourchet/commander"
	"github.com/apourchet/kodder/lib/client"
	"github.com/uber/makisu/lib/fileio"
	"github.com/uber/makisu/lib/log"
	"github.com/uber/makisu/lib/utils"
)

// ClientApplication is the struct that can talk to the Kodder worker through a socket or HTTP.
type ClientApplication struct {
	BuildFlags `commander:"flagstruct=build"`

	LocalSharedPath  string  `commander:"flag=local,The absolute path of the local mountpoint shared with the makisu worker."`
	WorkerSharedPath string  `commander:"flag=shared,The absolute destination of the mountpoint shared with the makisu worker."`
	HTTPAddress      string  `commander:"flag=address,The address of the Kodder worker."`
	Socket           *string `commander:"flag=socket,The absolute path of the unix socket that the makisu worker listens on."`

	WaitDuration time.Duration `commander:"flag=wait,The time to wait for worker to ready up (valid for build and abort calls). Format follows the golang time.Duration spec."`
}

// BuildFlags are the flags that the build command can take in.
type BuildFlags struct {
	Dockerfile *string `commander:"flag=f,Path to the dockerfile to build."`
	Tag        string  `commander:"flag=t,image tag (required)"`

	Arguments      map[string]string `commander:"flag=build-args,Arguments to the dockerfile as per the spec of ARG. Format is a json object."`
	Destination    string            `commander:"flag=dest,Destination of the image tar."`
	PushRegistries string            `commander:"flag=push,Push image after build to the comma-separated list of registries."`
	RegistryConfig string            `commander:"flag=registry-config,Registry configuration file for pulling and pushing images. Default configuration for DockerHub is used if not specified."`

	AllowModifyFS bool   `commander:"flag=modifyfs,Allow makisu to touch files outside of its own storage dir."`
	StorageDir    string `commander:"flag=storage,Directory that makisu uses for temp files and cached layers. Mount this path for better caching performance. If modifyfs is set, default to /makisu-storage; Otherwise default to /tmp/makisu-storage."`
	Blacklist     string `commander:"flag=blacklist,Comma separated list of files/directories. Makisu will omit all changes to these locations in the resulting docker images."`

	DockerHost    string `commander:"flag=docker-host,Docker host to load images to."`
	DockerVersion string `commander:"flag=docker-version,Version string for loading images to docker."`
	DockerScheme  string `commander:"flag=docker-scheme,Scheme for api calls to docker daemon."`
	DoLoad        bool   `commander:"flag=load,Load image after build."`

	RedisCacheAddress   string `commander:"flag=redis-cache-addr,The address of a redis cache server for cacheID to layer sha mapping."`
	CacheTTL            int    `commander:"flag=cache-ttl,The TTL of cacheID-sha mapping entries in seconds"`
	CompressionLevelStr string `commander:"flag=compression,Image compression level, could be 'no', 'speed', 'size', 'default'."`
	Commit              string `commander:"flag=commit,Set to explicit to only commit at steps with '#!COMMIT' annotations; Set to implicit to commit at every ADD/COPY/RUN step."`
}

var defaultDockerfilePath = ".kodder.dockerfile"

// NewClientApplication returns a new ClientApplication to talk to the Kodder worker.
func NewClientApplication() *ClientApplication {
	return &ClientApplication{
		BuildFlags: BuildFlags{
			Arguments: map[string]string{},

			AllowModifyFS: true,
			StorageDir:    "",

			DockerHost:    utils.DefaultEnv("DOCKER_HOST", "unix:///var/run/docker.sock"),
			DockerVersion: utils.DefaultEnv("DOCKER_VERSION", "1.21"),
			DockerScheme:  utils.DefaultEnv("DOCKER_SCHEME", "http"),

			RedisCacheAddress:   "",
			CacheTTL:            7 * 24 * 3600,
			CompressionLevelStr: "default",

			Commit: "implicit",
		},
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
func (app *ClientApplication) Build(context string) error {
	cleanup, err := app.placeDockerfile(context)
	if err != nil {
		return fmt.Errorf("failed to move dockerfile into worker context: %v", err)
	}
	defer cleanup()

	app.Dockerfile = &defaultDockerfilePath
	flags, err := commander.New().GetFlagSet(app.BuildFlags, "kodder build")
	if err != nil {
		return err
	}
	args := flags.Stringify()

	target, err := prepContext(app.LocalSharedPath, app.WorkerSharedPath, context)
	if err != nil {
		return fmt.Errorf("failed to prepare context: %v", err)
	}
	localPath := filepath.Join(app.LocalSharedPath, target)
	defer os.RemoveAll(localPath)
	workerPath := filepath.Join(app.WorkerSharedPath, target)

	start := time.Now()
	for time.Since(start) < app.WaitDuration {
		if err = app.client().Build(args, workerPath); err == client.ErrWorkerBusy {
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

func (app *ClientApplication) placeDockerfile(context string) (func(), error) {
	src := filepath.Join(context, "Dockerfile")
	if app.Dockerfile != nil {
		src = *app.Dockerfile
	}

	uid, gid, err := utils.GetUIDGID()
	if err != nil {
		return nil, fmt.Errorf("failed to get uid and gid for dockerfile move: %v", err)
	}

	dest := filepath.Join(context, ".kodder.dockerfile")
	cleanup := func() { os.Remove(dest) }
	return cleanup, fileio.NewCopier(nil).CopyFile(src, dest, uid, gid)
}

func (app *ClientApplication) client() *client.KodderClient {
	if app.Socket == nil {
		return client.NewWithAddress(app.HTTPAddress)
	}
	return client.NewWithSocket(*app.Socket)
}
