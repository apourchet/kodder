FROM golang:1.11.1
ADD . $GOPATH/src/github.com/apourchet/kodder
WORKDIR $GOPATH/src/github.com/apourchet/kodder
RUN make && cp bin/kodder /kodder

FROM gcr.io/makisu-project/makisu:v0.1.3
COPY --from=0 /kodder /makisu-internal/kodder
ENTRYPOINT ["/makisu-internal/kodder", "listen"]
