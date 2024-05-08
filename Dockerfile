# Stage 1: Build the Go binary
FROM golang:1.21.2-alpine AS builder

RUN apk add --no-cache git gcc musl-dev sqlite-dev
# Set the current working directory inside the container
WORKDIR /app
# Copy the source from the current directory to the working directory inside the container 
COPY server.go /app/server.go
COPY go.mod /app/go.mod
COPY go.sum /app/go.sum
COPY asserts /app/asserts
COPY README.md /app/README.md
COPY LICENSE /app/LICENSE
COPY VERSION /app/VERSION
COPY CHANGELOG.md /app/CHANGELOG.md
# Download all the dependencies
RUN go mod download
# Build the binary
# RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -o server ./server.go
RUN CGO_ENABLED=0 go build -tags 'netgo osusergo' -ldflags '-extldflags "-static"' -o server ./*.go

# Stage2: Use the UPX to optimize the size of the binary
FROM gruebel/upx:latest as upx
COPY --from=builder /app/server /app/server.org
RUN upx --best --lzma /app/server.org -o /app/server

# Stage 3: Run the binary in a minimal image
FROM alpine:latest
# Set the current working directory
WORKDIR /root/
# Copy the binary from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/server .
# Expose the application on port 8080
EXPOSE 8080
# Command to run the binary
CMD ["bash"]
