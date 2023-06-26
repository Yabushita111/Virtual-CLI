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

	"github.com/pkg/browser"
	log "github.com/spf13/jwalterweatherman"
)

var events chan GameEvent

type GameEvent struct {
	MyEvent string `json:"myData"`
}

// open gamelog.json
var gameID = "8d25c551-d275-4fb5-948e-2baa48f32a7a"
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
	// config handler and run server
	mux := http.NewServeMux()
	boardServer := BoardServer{
		events: make(chan GameEvent, 1000), // buffered channel to allow game to run ahead of browser client
		done:   make(chan bool),
		httpServer: &http.Server{
			Handler: cors.Default().Handler(mux),
		},
	}
	fmt.Println("config handle url")
	mux.HandleFunc("/games/"+gameID, boardServer.handleGame)
	mux.HandleFunc("/games/"+gameID+"/events", boardServer.handleWebsocket)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	go func() {
		err = boardServer.httpServer.Serve(listener)
		if err != http.ErrServerClosed {
			log.ERROR.Printf("Error in board HTTP server: %v", err)
		}
	}()
	// open browser
	board := "http://127.0.0.1:3000"
	serverURL := "http://" + listener.Addr().String()
	boardURL := fmt.Sprintf(board+"?engine=%s&game=%s&autoplay=true", serverURL, gameID)
	log.INFO.Printf("Opening board URL: %s", boardURL)
	if err := browser.OpenURL(boardURL); err != nil {
		log.ERROR.Printf("Failed to open browser: %v", err)
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// for http request
func (server *BoardServer) handleGame(w http.ResponseWriter, r *http.Request) {
	fmt.Println("send http message")
	w.Header().Add("Content-Type", "application/json")
	var file, _ = os.Open(filename)
	var scanner = bufio.NewScanner(file)
	scanner.Scan()
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
	json.NewDecoder(strings.NewReader(scanner.Text())).Decode(&server.events)
	for event := range server.events {
		jsonStr, err := json.Marshal(event)
		if err != nil {
			log.ERROR.Printf("Unable to serialize event for websocket: %v", err)
		}

		err = ws.WriteMessage(websocket.TextMessage, jsonStr)
		if err != nil {
			log.ERROR.Printf("Unable to write to websocket: %v", err)
			break
		}
	}

	log.DEBUG.Printf("Finished writing all game events, signalling game server to stop")
	close(server.done)

	log.DEBUG.Printf("Sending websocket close message")
	err = ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	if err != nil {
		log.ERROR.Printf("Problem closing websocket: %v", err)
	}
}
