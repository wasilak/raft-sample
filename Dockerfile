FROM golang:1.19-alpine

# Set the Current Working Directory inside the container
WORKDIR /app

COPY ./src .

# RUN go mod tidy

# Build the Go app
# RUN GO111MODULE=on CGO_ENABLED=0 GOOS=linux go build -mod=mod -o raftsample .
RUN go build -o /raftsample .
CMD ["/raftsample"]

# FROM scratch
# COPY --from=0 /raftsample /raftsample
# COPY --from=0 /app/raft.yml /raft.yml
# CMD ["/raftsample"]
