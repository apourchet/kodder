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

func (app *DaemonApplication) ready(rw http.ResponseWriter, req *http.Request) {
	if app.building.Load() {
		rw.WriteHeader(http.StatusConflict)
		return
	}
	rw.WriteHeader(http.StatusOK)
}

func (app *DaemonApplication) exit(rw http.ResponseWriter, req *http.Request) {
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

func (app *DaemonApplication) abort(rw http.ResponseWriter, req *http.Request) {
	if !app.building.Load() {
		rw.WriteHeader(http.StatusOK)
		return
	}
	rw.WriteHeader(http.StatusOK)
	app.aborting.Store(true)
}

func (app *DaemonApplication) build(rw http.ResponseWriter, req *http.Request) {
	if ok := app.building.CAS(false, true); !ok {
		rw.WriteHeader(http.StatusConflict)
		rw.Write([]byte("Already processing a request"))
		return
	}
	app.aborting.Store(false)
	defer app.building.Store(false)

	var fl http.Flusher
	if f, ok := rw.(http.Flusher); ok {
		fl = f
	}

	args, err := app.getBuildRequest(req)
	if err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(rw, "%s\n", err.Error())
		return
	}
	log.Infof("Build arguments passed in: %+v", args)

	cmd, outR, errR, err := prepCommand(args)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(rw, "%s\n", err.Error())
		return
	}

	if err := cmd.Start(); err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(rw, "%s\n", err.Error())
		return
	}
	rw.WriteHeader(http.StatusOK)

	go flushLines(outR, rw, fl)
	go flushLines(errR, rw, fl)

	done := atomic.NewBool(false)
	go app.listenForAbort(cmd, done)

	var exitCode int
	if err := cmd.Wait(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			// Command exited with code other than 0.
			ws := exitError.Sys().(syscall.WaitStatus)
			exitCode = ws.ExitStatus()
		}
	}
	done.Store(true)
	fmt.Fprintf(rw, `{"build_code": "%d"}`+"\n", exitCode)

	if err := app.cleanup(); err != nil {
		fmt.Fprintf(rw, `{"reset_error": %v}`+"\n", err)
		log.Errorf("Cleanup failed, exiting: %v", err)
		os.Exit(1)
		return
	}
	log.Infof("Build finished, exit code: %d", exitCode)
}

func (app *DaemonApplication) listenForAbort(cmd *exec.Cmd, done *atomic.Bool) {
	for {
		time.Sleep(1 * time.Second)
		if app.aborting.Load() {
			break
		} else if done.Load() {
			return
		}
	}
	log.Warnf("Aborting build")
	if err := cmd.Process.Kill(); err != nil {
		log.Errorf("failed to kill process: %v", err)
	}
}
