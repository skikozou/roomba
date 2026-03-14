package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/websocket"
	"github.com/tarm/serial"
)

//go:embed static
var staticFiles embed.FS

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <port>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "example: %s /dev/pts/4\n", os.Args[0])
		os.Exit(1)
	}

	portName := os.Args[1]
	c := &serial.Config{
		Name:        portName,
		Baud:        115200,
		ReadTimeout: 500,
	}
	conn, err := serial.OpenPort(c)
	if err != nil {
		log.Fatalf("failed to open serial port %s: %v", portName, err)
	}
	defer conn.Close()

	roomba := NewRoomba(conn)
	roomba.Start()
	roomba.Safe()
	log.Printf("Roomba connected on %s", portName)

	// 静的ファイル配信
	sub, _ := fs.Sub(staticFiles, "static")
	http.Handle("/", http.FileServer(http.FS(sub)))

	// WebSocketエンドポイント：受け取ったバイナリをそのままシリアルに流す
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		wsConn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("websocket upgrade error: %v", err)
			return
		}
		defer wsConn.Close()
		log.Printf("client connected: %s", r.RemoteAddr)

		for {
			_, msg, err := wsConn.ReadMessage()
			if err != nil {
				log.Printf("client disconnected: %v", err)
				// 切断時に停止
				conn.Write([]byte{145, 0, 0, 0, 0})
				break
			}
			if _, err := conn.Write(msg); err != nil {
				log.Printf("serial write error: %v", err)
			}
		}
	})

	addr := ":8080"
	log.Printf("server listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

