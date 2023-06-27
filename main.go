package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/rs/cors"

	log "github.com/spf13/jwalterweatherman"
)

type GameEventType string

type GameEvent struct {
	EventType GameEventType `json:"Type"`
	Data      interface{}   `json:"Data"`
}

// arg as gameID
var gameID = os.Args[1]
var battlelogPath = "/Users/yabu/Battlesnake-rules/cli/battlesnake/battlelog/"
var filename = battlelogPath + gameID + ".json"

//var scanner = bufio.NewScanner(file)
//scanner.Scan()

type Game struct {
	ID           string            `json:"ID"`
	Status       string            `json:"Status"`
	Width        int               `json:"Width"`
	Height       int               `json:"Height"`
	Ruleset      map[string]string `json:"Ruleset"`
	SnakeTimeout int               `json:"SnakeTimeout"`
	Source       string            `json:"Source"`
	RulesetName  string            `json:"RulesetName"`
	RulesStages  []string          `json:"RulesStages"`
	Map          string            `json:"Map"`
}

type BoardServer struct {
	game   Game
	events chan GameEvent // channel for sending events from the game runner to the browser client
	done   chan bool      // channel for signalling (via closing) that all events have been sent to the browser client

	httpServer *http.Server
}

func main() {
	// print browser url
	board := "http://127.0.0.1:3000/"
	vcli := "http://127.0.0.1:8080"
	boardURL := board + "?engine=" + vcli + "&game=" + gameID
	fmt.Println(boardURL)
	// config handler and run server
	mux := http.NewServeMux()
	boardServer := BoardServer{
		events: make(chan GameEvent, 1000), // buffered channel to allow game to run ahead of browser client
		done:   make(chan bool),
		httpServer: &http.Server{
			Handler: cors.Default().Handler(mux),
		},
	}
	mux.HandleFunc("/games/"+gameID, boardServer.handleGame)
	mux.HandleFunc("/games/"+gameID+"/events", boardServer.handleWebsocket)
	listener, _ := net.Listen("tcp", ":8080")
	boardServer.httpServer.Serve(listener)
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// for http request
func (server *BoardServer) handleGame(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "application/json")
	var file, _ = os.Open(filename)
	var scanner = bufio.NewScanner(file)
	scanner.Scan()
	// jsonをgame型にdecodeしてもう一度jsonにencodeしている.うまいことscanner.textからpostしたいが...
	json.NewDecoder(strings.NewReader(scanner.Text())).Decode(&server.game)
	err := json.NewEncoder(w).Encode(struct {
		Game Game
	}{server.game})
	if err != nil {
		log.ERROR.Printf("Unable to serialize game: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

// for websocket
func (server *BoardServer) handleWebsocket(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.ERROR.Printf("Unable to upgrade connection: %v", err)
		return
	}
	defer func() {
		err = ws.Close()
		if err != nil {
			log.ERROR.Printf("Unable to close websocket stream")
		}
	}()
	var file, _ = os.Open(filename)
	var scanner = bufio.NewScanner(file)
	scanner.Scan()
	var events []GameEvent
	for scanner.Scan() {
		var event GameEvent
		// jsonをgame型にdecodeしてもう一度jsonにencodeしている.うまいことscanner.textからpostしたいが...
		json.NewDecoder(strings.NewReader(scanner.Text())).Decode(&event)
		events = append(events, event)
	}
	for _, event := range events {
		jsonStr, _ := json.Marshal(event)
		ws.WriteMessage(websocket.TextMessage, jsonStr)
	}
	log.DEBUG.Printf("Finished writing all game events, signalling game server to stop")

	err = ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	if err != nil {
		log.ERROR.Printf("Problem closing websocket: %v", err)
	}
}
