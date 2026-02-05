FROM golang:1.25 AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /family-app ./cmd/family-app

FROM gcr.io/distroless/base-debian12

WORKDIR /app

COPY --from=build /family-app /app/family-app
COPY migrations /app/migrations

ENV HTTP_PORT=8080

EXPOSE 8080

ENTRYPOINT ["/app/family-app"]
