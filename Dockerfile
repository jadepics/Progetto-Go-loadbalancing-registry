# syntax=docker/dockerfile:1

FROM golang:1.22 AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download

COPY . .
ARG CMD=registry
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${CMD}

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
EXPOSE 9000 9101 9102 9201 9202
ENTRYPOINT ["/app"]
