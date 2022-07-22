FROM golang:1.18-alpine

# Set the Current Working Directory inside the container
WORKDIR /app

COPY . .

RUN go mod tidy

# Build the Go app
RUN GO111MODULE=on CGO_ENABLED=0 GOOS=linux go build -mod=mod -o raftsample .

FROM scratch
COPY --from=0 /app/raftsample /raftsample
CMD ["/raftsample"]
