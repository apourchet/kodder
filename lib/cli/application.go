package cli

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/uber/makisu/lib/log"
	"go.uber.org/atomic"
)

type Application struct {
	ListenFlags `commander:"flagstruct=listen"`
	BuildFlags  `commander:"flagstruct=build"`
	RunFlags    `commander:"flagstruct=run"`
	building    *atomic.Bool
}

func NewApplication() *Application {
	return &Application{
		ListenFlags: NewListenFlags(),

		building: atomic.NewBool(false),
	}
}

func (app *Application) CommanderDefault() error {
	return fmt.Errorf("need one of `build`, `run` or `listen` as subcommand")
}

func (app *Application) Listen() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ready", app.ready)
	mux.HandleFunc("/exit", app.exit)
	mux.HandleFunc("/build", app.build)

	lis, err := app.ListenFlags.getListener()
	if err != nil {
		return fmt.Errorf("failed to get listener: %v", err)
	}

	server := http.Server{Handler: mux}
	if err := server.Serve(lis); err != nil {
		return fmt.Errorf("failed to serve on unix socket: %v", err)
	}
	return nil
}

func (app *Application) ready(rw http.ResponseWriter, req *http.Request) {
	if app.building.Load() {
		rw.WriteHeader(http.StatusConflict)
		return
	}
	rw.WriteHeader(http.StatusOK)
}

func (app *Application) exit(rw http.ResponseWriter, req *http.Request) {
	if ok := app.building.CAS(false, true); !ok {
		rw.WriteHeader(http.StatusConflict)
		rw.Write([]byte("Already processing a request"))
		return
	}
	rw.WriteHeader(http.StatusOK)
	go func() {
		time.Sleep(1 * time.Second)
		os.Exit(0)
	}()
}

func (app *Application) build(rw http.ResponseWriter, req *http.Request) {
	if ok := app.building.CAS(false, true); !ok {
		rw.WriteHeader(http.StatusConflict)
		rw.Write([]byte("Already processing a request"))
		return
	}
	defer app.building.Store(false)

	var fl http.Flusher
	if f, ok := rw.(http.Flusher); ok {
		fl = f
	}

	args, err := app.ListenFlags.getBuildRequest(req)
	if err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(rw, "%s\n", err.Error())
		return
	}
	log.Infof("Build arguments passed in: %+v", args)

	args = append([]string{"build"}, args...)
	cmd := exec.Command("/makisu-internal/makisu", args...)
	outR, err := cmd.StdoutPipe()
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(rw, "%s\n", err.Error())
		return
	}
	defer outR.Close()

	errR, err := cmd.StderrPipe()
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(rw, "%s\n", err.Error())
		return
	}
	defer errR.Close()

	if err := cmd.Start(); err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(rw, "%s\n", err.Error())
		return
	}
	rw.WriteHeader(http.StatusOK)

	go flushLines(outR, rw, fl)
	go flushLines(errR, rw, fl)

	var exitCode int
	if err := cmd.Wait(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			// Command exited with code other than 0.
			ws := exitError.Sys().(syscall.WaitStatus)
			exitCode = ws.ExitStatus()
		}
	}
	fmt.Fprintf(rw, `{"build_code": %d}\n`, exitCode)
}
