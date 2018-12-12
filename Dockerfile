FROM golang:1.11.1
ADD . $GOPATH/src/github.com/apourchet/kodder
WORKDIR $GOPATH/src/github.com/apourchet/kodder
RUN make && cp bin/kodderd /kodderd

FROM gcr.io/makisu-project/makisu:v0.1.6
COPY --from=0 /kodderd /makisu-internal/kodderd
ENTRYPOINT ["/makisu-internal/kodderd"]
