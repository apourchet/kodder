package cli

import (
	"bufio"
	"io"
	"net/http"
	"os"
)

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
