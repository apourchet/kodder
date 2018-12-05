package cli

import (
	"fmt"
	"net"
	"os"
	"path"

	"github.com/uber/makisu/lib/log"
)

type ListenFlags struct {
	Socket  string `commander:"flag=socket,The path to the socket that Kodder will listen on for build requests"`
	Replace bool   `commander:"flag=replace,Whether or not to remove an existing socket at the same path"`

	Port *int `commander:"flag=port,The port that Kodder will listen on for build requests"`
}

func NewListenFlags() ListenFlags {
	return ListenFlags{
		Socket:  defaultEnv("KODDER_SOCKET", "/kodder/kodder.sock"),
		Replace: false,
	}
}

func (flags ListenFlags) getListener() (net.Listener, error) {
	if flags.Port != nil {
		log.Infof("Listening for build requests on port: %d", *flags.Port)
		return net.Listen("tcp", fmt.Sprintf(":%d", *flags.Port))
	}

	if err := os.MkdirAll(path.Dir(flags.Socket), os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create directory to socket %s: %v", flags.Socket, err)
	}

	if _, err := os.Stat(flags.Socket); flags.Replace && !os.IsNotExist(err) {
		if err := os.Remove(flags.Socket); err != nil {
			return nil, fmt.Errorf("failed to replace existing socket: %v", err)
		}
	}

	log.Infof("Listening for build requests on unix socket %s", flags.Socket)
	return net.Listen("unix", flags.Socket)
}

type BuildFlags struct{}

type RunFlags struct{}
