package cli

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path"

	"github.com/uber/makisu/lib/log"
)

// BuildRequest is the expected structure of the JSON body of http requests coming in on the socket.
// Example body of a BuildRequest:
//    ["build", "-t", "myimage:latest", "/context"]
type BuildRequest []string

func (app *DaemonApplication) getListener() (net.Listener, error) {
	if app.Port != nil {
		log.Infof("Listening for build requests on port: %d", *app.Port)
		return net.Listen("tcp", fmt.Sprintf(":%d", *app.Port))
	}

	if err := os.MkdirAll(path.Dir(app.Socket), os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create directory to socket %s: %v", app.Socket, err)
	}

	if _, err := os.Stat(app.Socket); app.Replace && !os.IsNotExist(err) {
		if err := os.Remove(app.Socket); err != nil {
			return nil, fmt.Errorf("failed to replace existing socket: %v", err)
		}
	}

	log.Infof("Listening for build requests on unix socket %s", app.Socket)
	return net.Listen("unix", app.Socket)
}

func (app *DaemonApplication) getBuildRequest(req *http.Request) (BuildRequest, error) {
	args := BuildRequest{}
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return args, err
	} else if err := json.Unmarshal(body, &args); err != nil {
		return args, err
	}
	return args, nil
}
