FROM golang:1.12.1
ADD . /workspace
WORKDIR /workspace
RUN make

FROM gcr.io/makisu-project/makisu:v0.1.11
COPY --from=0 /workspace/bin/kodderd /makisu-internal/kodderd
ENTRYPOINT ["/makisu-internal/kodderd"]
