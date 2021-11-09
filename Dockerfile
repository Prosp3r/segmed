# # #FROM golang:1.12.0-alpine3.9
# FROM golang:alpine 
# RUN apk add git
# RUN mkdir /app
# ADD . /app
# WORKDIR /app
# RUN ls -aslt
# RUN go build -o segmed
# RUN ls -aslt /app
# EXPOSE 8080
# CMD ["/app/segmed"]


FROM golang:1.15-alpine as builder
RUN mkdir /build 
ADD . /build/
COPY . /build/
WORKDIR /build 
COPY go.mod go.sum ./
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-extldflags "-static"' -o main .
FROM scratch
COPY --from=builder /build/* /app/
WORKDIR /app

EXPOSE 8080
CMD ["./main"]