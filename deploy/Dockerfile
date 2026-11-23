FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go VERSION ./
COPY web ./web
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(cat VERSION)" -o /mediathek .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
COPY --from=build /mediathek /usr/local/bin/mediathek
ENTRYPOINT ["mediathek","-account-host","-account-listen","0.0.0.0:8765","-data","/accounts"]
