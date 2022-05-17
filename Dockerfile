FROM golang:1.18
WORKDIR /root
COPY . .
RUN cd api && CGO_ENABLED=0 go build -o /root/api_server -v ./...

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root
COPY --from=0 /root/api_server .
CMD ["./api_server"]