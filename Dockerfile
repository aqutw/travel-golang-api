FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -tags=nomsgpack -trimpath -o /travel-api ./cmd/server

FROM scratch
WORKDIR /app
COPY --from=build /travel-api /app/travel-api
COPY --chown=65532:65532 data /app/data
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/app/travel-api"]
