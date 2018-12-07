package cli

import (
	"bufio"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/uber/makisu/lib/fileio"
	"github.com/uber/makisu/lib/log"
	"github.com/uber/makisu/lib/utils"
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

// Takes in the local path of the context, copies the files to a new directory inside the worker's
// mount namespace and returns the context path inside the shared mount location.
// Example: prepareContext("/home/joe/test/context") => context-12345
// This means that the context was copied over to <localShared>/context-12345
func prepContext(localShared, workerShared, context string) (string, error) {
	context, err := filepath.Abs(context)
	if err != nil {
		return "", err
	}

	rand := rand.New(rand.NewSource(time.Now().Unix()))
	targetContext := fmt.Sprintf("context-%d", rand.Intn(10000))
	targetPath := filepath.Join(localShared, targetContext)

	uid, gid, err := utils.GetUIDGID()
	if err != nil {
		return "", err
	}

	log.Infof("Copying context to worker filesystem: %s => %s", context, targetPath)
	start := time.Now()
	if err := fileio.NewCopier(nil).CopyDir(context, targetPath, uid, gid); err != nil {
		return "", err
	}
	log.Infof("Finished copying over context in %v", time.Since(start))
	return targetContext, nil
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
