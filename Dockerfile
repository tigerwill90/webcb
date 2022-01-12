FROM golang:1.17.6-alpine3.15 as builder

WORKDIR /app

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o webcb main.go

FROM gcr.io/distroless/static-debian11

COPY --from=builder /app/webcb .
COPY --from=builder --chown=65532:65532 /opt /opt

ENTRYPOINT ["/webcb"]

USER 65532
EXPOSE 4444