FROM golang:1.18-alpine
WORKDIR /app

COPY go.mod ./
COPY go.sum ./
COPY *.go ./
RUN go mod download & \
    go mod tidy & \
    go build -o /otlp-log-reciever
CMD [ "/otlp-log-reciever" ]