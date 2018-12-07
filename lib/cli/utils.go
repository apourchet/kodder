package cli

import (
	"bufio"
	"io"
	"net/http"
	"os"
	"os/exec"
)

var RootLevelSkips = map[string]bool{
	"/proc":            true,
	"/sys":             true,
	"/dev":             true,
	"/makisu-internal": true,
	"/makisu-storage":  true,
}

var MustRemove = []string{
	"/makisu-storage/sandbox",
}

func defaultEnv(key, value string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return value
}

func flushLines(r io.Reader, w io.Writer, fl http.Flusher) {
	reader := bufio.NewReader(r)
	for {
		line, _, err := reader.ReadLine()
		if err == io.EOF {
			return
		} else if err != nil {
			return
		}
		line = append(line, '\n')
		w.Write(line)
		if fl != nil {
			fl.Flush()
		}
	}
}

func prepCommand(args []string) (cmd *exec.Cmd, outR io.ReadCloser, errR io.ReadCloser, err error) {
	args = append([]string{"build"}, args...)
	cmd = exec.Command("/makisu-internal/makisu", args...)
	outR, err = cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, err
	}

	errR, err = cmd.StderrPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	return cmd, outR, errR, nil
}
