package cli

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/uber/makisu/lib/log"
	"github.com/uber/makisu/lib/mountutils"
	"go.uber.org/atomic"
)

type DaemonApplication struct {
	Replace bool    `commander:"flag=replace,Whether or not to remove an existing socket at the same path"`
	Port    int     `commander:"flag=port,The port that Kodder will listen on for build requests"`
	Socket  *string `commander:"flag=socket,The path to the socket that Kodder will listen on for build requests"`

	building *atomic.Bool

	// originals is the list of files that were
	// there when the app was started.
	originals map[string]bool
}

func NewDaemonApplication() *DaemonApplication {
	return &DaemonApplication{
		Port:    3456,
		Replace: false,

		building:  atomic.NewBool(false),
		originals: map[string]bool{},
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
	log.Infof("Cleaning up filesystem")
	root, err := os.Open("/")
	if err != nil {
		return fmt.Errorf("cleanup error: %v", err)
	}
	infos, err := root.Readdir(-1)
	if err != nil {
		return fmt.Errorf("cleanup error: %v", err)
	}

	trash := map[string]bool{}
	for _, info := range infos {
		fname := filepath.Join("/", info.Name())
		if _, found := RootLevelSkips[fname]; found {
			continue
		} else if skip, err := mountutils.IsMounted(fname); err != nil {
			return fmt.Errorf("failed to check mountpoints: %v", err)
		} else if skip {
			log.Infof("Skipping cleanup of %v, mountpoint detected", fname)
			continue
		}

		fi, err := os.Stat(fname)
		if err != nil {
			return fmt.Errorf("failed to stat file to remove %v: %v", fname, err)
		} else if _, found := app.originals[fname]; found && !fi.IsDir() {
			log.Infof("Skipping cleanup of %v, present at container launch", fname)
			continue
		}

		if !fi.IsDir() {
			if err := os.RemoveAll(fname); err != nil {
				return fmt.Errorf("failed to cleanup %v: %v", fname, err)
			}
			continue
		}

		err = filepath.Walk(fname, func(path string, fi os.FileInfo, err error) error {
			if _, found := app.originals[path]; found {
				return nil
			} else if mounted, _ := mountutils.IsMounted(path); mounted {
				log.Infof("Path mounted: %v", path)
				return nil
			} else if mountpoint, _ := mountutils.IsMountpoint(path); mountpoint {
				log.Infof("Mountpoint detected: %v", path)
				return filepath.SkipDir
			} else if contains, _ := mountutils.ContainsMountpoint(path); contains {
				log.Infof("Inner mountpoint detected: %v", path)
				return nil
			}
			trash[path] = true
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to cleanup directory: %v", err)
		}
	}

	for _, fname := range MustRemove {
		trash[fname] = true
	}

	trashSlice := []string{}
	for fname := range trash {
		trashSlice = append(trashSlice, fname)
	}
	sort.Strings(trashSlice)
	for i := len(trashSlice) - 1; i >= 0; i-- {
		fname := trashSlice[i]
		if err := os.RemoveAll(fname); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to cleanup directory: %v", err)
		}
	}
	return nil
}
