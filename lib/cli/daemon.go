package cli

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/uber/makisu/lib/log"
	"github.com/uber/makisu/lib/mountutils"
	"go.uber.org/atomic"
)

type DaemonApplication struct {
	Socket  string `commander:"flag=socket,The path to the socket that Kodder will listen on for build requests"`
	Replace bool   `commander:"flag=replace,Whether or not to remove an existing socket at the same path"`
	Port    *int   `commander:"flag=port,The port that Kodder will listen on for build requests"`

	building *atomic.Bool

	// originals is the list of files/directories that were
	// there when the app was started.
	originals map[string]bool
}

func NewDaemonApplication() *DaemonApplication {
	return &DaemonApplication{
		Socket:  defaultEnv("KODDER_SOCKET", "/kodder/kodder.sock"),
		Replace: false,

		building:  atomic.NewBool(false),
		originals: map[string]bool{"/makisu-storage": true},
	}
}

func (app *DaemonApplication) CommanderDefault() error {
	return app.listen()
}

func (app *DaemonApplication) listen() error {
	filepath.Walk("/", func(path string, fi os.FileInfo, err error) error {
		app.originals[path] = true
		return nil
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/ready", app.ready)
	mux.HandleFunc("/exit", app.exit)
	mux.HandleFunc("/build", app.build)

	lis, err := app.getListener()
	if err != nil {
		return fmt.Errorf("failed to get listener: %v", err)
	}

	server := http.Server{Handler: mux}
	if err := server.Serve(lis); err != nil {
		return fmt.Errorf("failed to serve on unix socket: %v", err)
	}
	return nil
}

func (app *DaemonApplication) cleanup() error {
	root, err := os.Open("/")
	if err != nil {
		return fmt.Errorf("cleanup error: %v", err)
	}
	infos, err := root.Readdir(-1)
	if err != nil {
		return fmt.Errorf("cleanup error: %v", err)
	}
	for _, info := range infos {
		fname := filepath.Join("/", info.Name())
		if skip, err := mountutils.ContainsMountpoint(fname); err != nil {
			return fmt.Errorf("failed to check mountpoints: %v", err)
		} else if skip {
			log.Debugf("Skipping cleanup of %v, mountpoint detected", fname)
			continue
		} else if _, found := app.originals[fname]; found {
			log.Debugf("Skipping cleanup of %v, present at container launch", fname)
			continue
		}

		log.Infof("Cleaning up dir: %v", fname)
		if err := os.RemoveAll(fname); err != nil {
			return fmt.Errorf("failed to cleanup %v: %v", fname, err)
		}
	}
	return nil
}
