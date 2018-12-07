package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/apourchet/commander"
	"github.com/apourchet/kodder/lib/client"
	"github.com/uber/makisu/lib/fileio"
	"github.com/uber/makisu/lib/log"
	"github.com/uber/makisu/lib/utils"
)

type ClientApplication struct {
	BuildFlags `commander:"flagstruct=build"`

	LocalSharedPath  string  `commander:"flag=local,The absolute path of the local mountpoint shared with the makisu worker."`
	WorkerSharedPath string  `commander:"flag=shared,The absolute destination of the mountpoint shared with the makisu worker."`
	HTTPAddress      string  `commander:"flag=address,The address of the Kodder worker."`
	Socket           *string `commander:"flag=socket,The absolute path of the unix socket that the makisu worker listens on."`
}

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
	}
}

func (app *ClientApplication) CommanderDefault() error {
	return fmt.Errorf("command needed; one of `ready` or `build`")
}

func (app *ClientApplication) Ready() error {
	if ready, err := app.client().Ready(); err != nil {
		return err
	} else if !ready {
		return fmt.Errorf("worker not ready")
	}
	log.Infof("Worker is ready")
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
	return app.client().Build(args, context)
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
		return client.NewWithAddress(app.HTTPAddress, app.LocalSharedPath, app.WorkerSharedPath)
	}
	return client.NewWithSocket(*app.Socket, app.LocalSharedPath, app.WorkerSharedPath)
}
