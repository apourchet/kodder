package cli

import (
	"fmt"

	"github.com/apourchet/kodder/lib/client"
	"github.com/uber/makisu/lib/log"
)

type ReadyFlags struct {
	LocalSharedPath  string `commander:"flag=l,The absolute path of the local mountpoint shared with the makisu worker"`
	WorkerSharedPath string `commander:"flag=w,The absolute destination of the mountpoint shared with the makisu worker"`

	SocketPath  string  `commander:"flag=s,The absolute path of the unix socket that the makisu worker listens on"`
	HTTPAddress *string `commander:"flag=address,The address of the Kodder worker."`
}

func NewReadyFlags() ReadyFlags {
	return ReadyFlags{
		SocketPath:       "/kodder/kodder.sock",
		LocalSharedPath:  "/kodder-context",
		WorkerSharedPath: "/kodder-context",
	}
}

func (flags ReadyFlags) client() *client.KodderClient {
	if flags.HTTPAddress != nil {
		return client.NewWithAddress(*flags.HTTPAddress, flags.LocalSharedPath, flags.WorkerSharedPath)
	}
	return client.NewWithSocket(flags.SocketPath, flags.LocalSharedPath, flags.WorkerSharedPath)
}

func (flags ReadyFlags) Ready() error {
	log.Infof("Ready?")
	if ready, err := flags.client().Ready(); err != nil {
		return err
	} else if !ready {
		return fmt.Errorf("worker not ready")
	}
	log.Infof("Worker is ready")
	return nil
}
