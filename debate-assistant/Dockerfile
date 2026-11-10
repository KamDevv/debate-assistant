FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/debate-api ./cmd/server
FROM alpine:3.21
WORKDIR /app
RUN adduser -D appuser && mkdir -p /app/data && chown -R appuser /app
COPY --from=build /out/debate-api /app/debate-api
USER appuser
EXPOSE 8080
CMD ["/app/debate-api"]
