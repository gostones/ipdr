###
FROM golang:1.14.4-alpine3.12 as builder

RUN apk add --no-cache bash gcc libc-dev make openssl-dev

WORKDIR /app

ARG CGO_ENABLED=0
ENV GO111MODULE=on
ENV GOPATH=

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN go fmt ./... \
    && go vet ./...
# RUN go build -o bin/ipdr ./cmd/ipdr
RUN make build

###
FROM alpine:3.12

COPY --from=builder /app/bin/ipdr /usr/local/bin/ipdr

EXPOSE 5000

ENV USER=ipdr

RUN adduser -D -h /home/$USER -u 1000 -G users $USER

USER $USER

CMD ["ipdr", "server", "--port", "5000"]
