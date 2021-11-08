# #FROM golang:1.12.0-alpine3.9
FROM golang:alpine 
RUN apk add git
RUN mkdir /app
ADD . /app
WORKDIR /app
RUN ls -aslt
RUN go build -o segmed
RUN ls -aslt /app
EXPOSE 8080
CMD ["/app/segmed"]