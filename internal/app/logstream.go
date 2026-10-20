package app

import (
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"

	"shipdude/pkg/socket"
)

// Upgrader upgrades inbound HTTP connections to WebSocket streams. Any origin
// is accepted because the portal is a single-operator, local-first tool.
var Upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// StreamJobLogs upgrades the connection and replays then tails a job's logs.
func StreamJobLogs(w http.ResponseWriter, r *http.Request, hub *socket.Hub, jobID string) {
	conn, err := Upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	job := hub.GetOrCreateJob(jobID)

	for _, line := range job.GetLogs() {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(line)); err != nil {
			return
		}
	}

	listenerID := fmt.Sprintf("%p", conn)
	ch := job.RegisterListener(listenerID)
	defer job.UnregisterListener(listenerID)

	for line := range ch {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(line)); err != nil {
			return
		}
	}
}
