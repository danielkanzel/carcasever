FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server

FROM alpine:3.22
WORKDIR /app
COPY --from=build /out/server /app/server
COPY web /app/web
COPY resources /app/resources
ENV WEB_DIR=/app/web
ENV ART_DIR=/app/resources
EXPOSE 8080
CMD ["/app/server"]
