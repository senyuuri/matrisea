FROM golang:1.16
WORKDIR /go/src/sea.com/matrisea
COPY . .
RUN cd api && go build -o ../api-server .

FROM alpine:latest  
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=0 /go/src/sea.com/matrisea/api-server .
CMD ["./api-server"]  