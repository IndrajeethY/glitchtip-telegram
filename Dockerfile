FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY main.go .
RUN CGO_ENABLED=0 go build -o /glitchtip-telegram .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /glitchtip-telegram /usr/local/bin/glitchtip-telegram
ENTRYPOINT ["glitchtip-telegram"]
