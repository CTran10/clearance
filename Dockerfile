FROM golang:1.25-alpine AS build

WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN go build -o /out/transaction-service ./cmd/transaction-service
RUN go build -o /out/outbox-publisher ./cmd/outbox-publisher
RUN go build -o /out/risk-service ./cmd/risk-service
RUN go build -o /out/ledger-service ./cmd/ledger-service
RUN go build -o /out/notification-service ./cmd/notification-service

FROM alpine:3.21

RUN apk add --no-cache ca-certificates wget
RUN adduser -D -H -u 10001 clearance
COPY --from=build /out/ /usr/local/bin/
USER clearance

EXPOSE 8080
