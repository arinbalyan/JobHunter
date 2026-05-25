# JobHunter — Go development environment
# ─────────────────────────────────────────────────────
FROM golang:1.26-alpine

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build ./...

CMD ["go", "run", "./cmd/doctor/"]
