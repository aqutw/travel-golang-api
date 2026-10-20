FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -o /travel-api ./cmd/server

FROM scratch
WORKDIR /app
COPY --from=build /travel-api /app/travel-api
COPY data/travel.sqlite3 /app/data/travel.sqlite3
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/app/travel-api"]
