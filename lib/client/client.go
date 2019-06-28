package client

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"

	"github.com/uber/makisu/lib/log"
)

// ErrWorkerBusy is returned when the worker is busy.
var ErrWorkerBusy = fmt.Errorf("Kodder worker busy")

// KodderClient is the struct that allows communication with a Kodder worker.
type KodderClient struct {
	WorkerLog func(line string)
	HTTPDo    func(req *http.Request) (*http.Response, error)
}

// NewWithSocket creates a new Kodder client that will talk to the worker available on the socket
// passed in.
func NewWithSocket(socket string) *KodderClient {
	return NewWithHTTP(&http.Transport{
		Dial: func(p, a string) (net.Conn, error) {
			return net.Dial("unix", socket)
		},
	})
}

// NewWithAddress creates a new Kodder client that will talk to the worker available at the address
// passed in.
func NewWithAddress(address string) *KodderClient {
	return NewWithHTTP(&http.Transport{
		Dial: func(p, a string) (net.Conn, error) {
			return net.Dial("tcp", address)
		},
	})
}

// NewWithHTTP returns a new Kodder client given an http transport.
func NewWithHTTP(transport *http.Transport) *KodderClient {
	cli := &http.Client{Transport: transport}
	return &KodderClient{
		WorkerLog: defaultWorkerLog,
		HTTPDo:    cli.Do,
	}
}

// SetWorkerLog sets the function called on every worker log line.
func (cli *KodderClient) SetWorkerLog(fn func(line string)) {
	cli.WorkerLog = fn
}

// Ready returns true if the worker is ready for work, and false if it is already performing
// a build.
func (cli *KodderClient) Ready() (bool, error) {
	req, err := http.NewRequest("GET", "http://localhost/ready", nil)
	if err != nil {
		return false, err
	}
	resp, err := cli.HTTPDo(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK, nil
}

// Exit tells the Kodder worker to exit cleanly.
func (cli *KodderClient) Exit() error {
	req, err := http.NewRequest("GET", "http://localhost/exit", nil)
	if err != nil {
		return err
	}
	resp, err := cli.HTTPDo(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status code from worker: %v", resp.StatusCode)
	}
	return nil
}

// Abort tells the Kodder worker to abort the current build.
func (cli *KodderClient) Abort() error {
	req, err := http.NewRequest("GET", "http://localhost/abort", nil)
	if err != nil {
		return err
	}
	resp, err := cli.HTTPDo(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status code from worker: %v", resp.StatusCode)
	}
	return nil
}

// Build kicks off a build on the Kodder worker at the context with the flags passed in.
func (cli *KodderClient) Build(flags []string, context string) error {
	args := append(flags, context)
	content, _ := json.Marshal(args)
	log.Infof("Sending build request to kodderd with args %v", args)

	reader := bytes.NewBuffer(content)
	req, err := http.NewRequest("POST", "http://localhost/build", reader)
	if err != nil {
		return err
	}

	resp, err := cli.HTTPDo(req)
	if err != nil {
		return err
	}
	if err := cli.readLines(resp.Body); err != nil {
		return err
	} else if resp.StatusCode == http.StatusOK {
		return nil
	} else if resp.StatusCode == http.StatusConflict {
		return ErrWorkerBusy
	}
	return fmt.Errorf("bad http status code from Kodder worker: %v", resp.StatusCode)
}

func (cli *KodderClient) readLines(body io.ReadCloser) error {
	var buildCode int
	defer body.Close()
	reader := bufio.NewReader(body)
	for {
		line, _, err := reader.ReadLine()
		if err == io.EOF {
			break
		} else if err != nil {
			return fmt.Errorf("failed to read build body: %v", err)
		}
		cli.WorkerLog(string(line))
		cli.maybeGetBuildCode(line, &buildCode)
	}
	if buildCode != 0 {
		return fmt.Errorf("build code returned was non-zero: %d", buildCode)
	}
	return nil
}

func (cli *KodderClient) maybeGetBuildCode(line []byte, code *int) {
	into := map[string]interface{}{}
	if err := json.Unmarshal(line, &into); err == nil {
		if val, found := into["build_code"]; found {
			if str, ok := val.(string); ok {
				if i, err := strconv.Atoi(str); err == nil {
					log.Infof("Got build exit code: %d", i)
					*code = i
				}
			}
		}
	}
}

func defaultWorkerLog(line string) {
	into := map[string]interface{}{}
	pipe := os.Stdout
	if err := json.Unmarshal([]byte(line), &into); err == nil {
		if val, found := into["level"]; found {
			if str, ok := val.(string); ok && str == "error" {
				pipe = os.Stderr
			}
		}
		if val, found := into["msg"]; found {
			if str, ok := val.(string); ok {
				line = str
			}
		}
	}
	fmt.Fprintf(pipe, line+"\n")
}
