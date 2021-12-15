############################################################
# Dockerfile to build gRPC-Go-Polaris quickstart example consumer

# First of all, you need to build the binary from the source code,
# use `CGO_ENABLED=0 go build -ldflags "-s -w" -o consumer` to build the binary.

############################################################

FROM golang:alpine

WORKDIR /root
ADD . /root

ENTRYPOINT ./consumer