FROM golang:1.16.2-alpine3.12

RUN apk add --no-cache git

# Add Maintainer Info
LABEL maintainer="Prosper O <sirpos@gmail.com>"

# Build Args
ARG HOME_DIR
ARG MODULE_NAME
ARG CMD_NAME
ARG LOG_DIR_NAME
ARG VOLUME_DIR
ARG VERSION
ARG PORT
ARG CGO_ENABLED

RUN echo "Build number:" . $VERSION

# Set the Current Working Directory inside the container
WORKDIR /

# Copy dependency files in first to take advantage of Docker caching.
COPY go.mod go.sum ./

# Get and install listed dependencies in one, rather than go get & go install.
RUN go mod download

# Copy everything else from the current directory to the PWD(Present Working Directory) inside the container
COPY . .

# WORKDIR /
RUN go build -o segmed .
EXPOSE 8080
CMD ["./segmed"]

# docker build -f Dockerfile -t segmed:latest .