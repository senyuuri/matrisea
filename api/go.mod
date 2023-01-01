module sea.com/matrisea/api

go 1.16

replace sea.com/matrisea/vmm => ../vmm

require (
	github.com/gin-contrib/cors v1.3.1
	github.com/gin-gonic/gin v1.8.2
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/gorilla/websocket v1.4.2
	sea.com/matrisea/vmm v0.0.0-00010101000000-000000000000
)
