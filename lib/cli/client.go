package cli

import (
	"fmt"

	"github.com/apourchet/kodder/lib/client"
	"github.com/uber/makisu/lib/log"
)

type ClientApplication struct {
	LocalSharedPath  string `commander:"flag=local,The absolute path of the local mountpoint shared with the makisu worker"`
	WorkerSharedPath string `commander:"flag=shared,The absolute destination of the mountpoint shared with the makisu worker"`

	Socket      string  `commander:"flag=socket,The absolute path of the unix socket that the makisu worker listens on"`
	HTTPAddress *string `commander:"flag=address,The address of the Kodder worker."`
}

func NewClientApplication() *ClientApplication {
	return &ClientApplication{
		Socket:           defaultEnv("KODDER_SOCKET", "/kodder/kodder.sock"),
		LocalSharedPath:  defaultEnv("KODDER_MNT_LOCAL", "/kodder/shared"),
		WorkerSharedPath: defaultEnv("KODDER_MNT_REMOTE", "/kodder/shared"),
	}
}

func (app *ClientApplication) CommanderDefault() error {
	return fmt.Errorf("command needed; one of `ready` or `build`")
}

func (app *ClientApplication) client() *client.KodderClient {
	if app.HTTPAddress != nil {
		return client.NewWithAddress(*app.HTTPAddress, app.LocalSharedPath, app.WorkerSharedPath)
	}
	return client.NewWithSocket(app.Socket, app.LocalSharedPath, app.WorkerSharedPath)
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
