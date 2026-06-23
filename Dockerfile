FROM golang:1.25-alpine AS build

WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN set -e; for dir in ./cmd/*; do go build -o "/out/$(basename "$dir")" "$dir"; done

FROM alpine:3.21

RUN apk add --no-cache ca-certificates wget
RUN adduser -D -H -u 10001 clearance
COPY --from=build /out/ /usr/local/bin/
USER clearance

EXPOSE 8080
