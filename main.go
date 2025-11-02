package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"sync"

	"time"

	"github.com/crgimenes/go-osc"
	"github.com/gorilla/websocket"
)

const deskAddr = "10.1.3.40:8001"
const localAddr = "0.0.0.0:9100"

const httpServerAddress = "0.0.0.0:8002"

func main() {
	localUdpAddr, _ := net.ResolveUDPAddr("udp", localAddr)
	deskUdpAddr, _ := net.ResolveUDPAddr("udp", deskAddr)

	d1 := osc.NewStandardDispatcher()
	app1 := osc.NewServerAndClient(d1)
	app1.NewConn(localUdpAddr, deskUdpAddr)

	// var wsConns = make(map[string]chan string)
	wsConns := make(map[string]chan EncodedWsMessage)
	wsConnsWrite := make(chan struct{uuid string; ch chan EncodedWsMessage}, 10)
	go func () {
		for {
			write := <- wsConnsWrite
			wsConns[write.uuid] = write.ch
		}
	}()

	d1.AddMsgHandler("*", func(msg *osc.Message) {
		addrFilter1 := regexp.MustCompile(`^(/channel/13)|(/console/pong)`)
		if addrFilter1.MatchString(msg.Address) {
			fmt.Printf("-> %v: %v \n", localUdpAddr, msg)
		}

		addrFilter := regexp.MustCompile(`.*`)
		if addrFilter.MatchString(msg.Address) {
			storeData[msg.Address] = msg.Arguments
			
			b, err := json.Marshal(WsMessage{msg.Address, msg.Arguments})
			if err != nil {
				println("error encoding to json: " + err.Error())
				return
			}

			for _, conn := range wsConns {
				conn <- b
			}
		}
	})

	go app1.ListenAndServe()

	// app1.SendMsg("/console/ping")

	maxCh := 64
	maxAux := 32
	for ch := 1; ch <= maxCh; ch++ {
		for aux := 1; aux <= maxAux; aux++ {
			app1.SendMsg("/sd/Input_Channels/" + strconv.Itoa(ch) + "/Aux_Send/" + strconv.Itoa(aux) + "/send_level/?")
			app1.SendMsg("/sd/Input_Channels/" + strconv.Itoa(ch) + "/Aux_Send/" + strconv.Itoa(aux) + "/send_pan/?")
		}
	}
	// app1.SendMsg("/console/resend")
	// app1.SendMsg("/channel/13/fader", float32(10.0))

	readChannelsData()
	httpChannelsDataRoutes()

	var upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	handleConnections := func (w http.ResponseWriter, r *http.Request)  {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Fatal(err)
		}
		defer ws.Close()

		var uuid string
		for {
			uuid = genUuid()
			if wsConns[uuid] == nil {
				break
			}
		}

		ch := make(chan EncodedWsMessage, 50)
		wsConnsWrite <- struct{uuid string; ch chan EncodedWsMessage}{uuid, ch}
		defer func(){ wsConns[uuid] = nil }()

		go func() {
			for {
				_, message, err := ws.ReadMessage()
				if err != nil {
					println(err.Error())
					break
				}
				fmt.Printf("Received: %s\n", message)

				var wsMessage WsMessage
				err = json.Unmarshal(message, &wsMessage)
				if err != nil {
					println(err.Error())
					continue
				}

				if (wsMessage.Address == "init") {
					for addr, args := range storeData {
						sendWsMessage := WsMessage{addr, args}
						ch <- encodeWs(sendWsMessage)
					}
				} else {
					storeDataLock.Lock()
					storeData[wsMessage.Address] = wsMessage.Args
					storeDataLock.Unlock()
					app1.SendMsg(wsMessage.Address, float32(wsMessage.Args[0].(float64)))
					go func() {
						for uuid2, ws2 := range wsConns {
							if uuid2 != uuid {
								ws2 <- message
							}
						}
					}()
				}
			}
		}()

		go func() {
			for {
				encodedWsMessage := <- ch
				if (encodedWsMessage != nil) {
					ws.WriteMessage(1, encodedWsMessage)
				}
			}
		}()

		for {
			time.Sleep(time.Minute)
		}
	}

	http.HandleFunc("/", handleConnections)
    log.Println("http server started on " + httpServerAddress)
    err := http.ListenAndServe(httpServerAddress, nil)
    if err != nil {
        log.Fatal("ListenAndServe: ", err)
    }

	for {
		time.Sleep(time.Second)
	}
}

func genUuid() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		log.Fatal(err)
	}
	uuid := fmt.Sprintf("%x-%x-%x-%x-%x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
	return uuid
}

type WsMessage struct {
	Address string `json:"address"`
	Args    []any  `json:"args"`
}

type EncodedWsMessage []byte

func encodeWs(msg WsMessage) EncodedWsMessage {
	b, err := json.Marshal(WsMessage{msg.Address, msg.Args})
	if err != nil {
		println("error encoding to json: " + err.Error())
		return nil
	}
	return b
}

var storeData = make(map[string][]any)
var storeDataLock = sync.Mutex{}
