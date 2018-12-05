package cli

import (
	"fmt"
	"net/http"
	"os"

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

func defaultEnv(key, value string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return value
}
