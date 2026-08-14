// Go Voice Agent Starter - Backend Server
//
// Simple WebSocket proxy to Deepgram's Voice Agent API.
// Forwards all messages (JSON and binary) bidirectionally between client and Deepgram.
//
// Routes:
//
//	GET  /api/session       - Issue signed session token
//	GET  /api/metadata      - Project metadata from deepgram.toml
//	WS   /api/voice-agent   - WebSocket proxy to Deepgram Agent API (auth required)
//	GET  /health            - Health check
package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	agentmsg "github.com/deepgram/deepgram-go-sdk/v3/pkg/api/agent/v1/websocket/interfaces"
	agent "github.com/deepgram/deepgram-go-sdk/v3/pkg/client/agent"
	dginterfaces "github.com/deepgram/deepgram-go-sdk/v3/pkg/client/interfaces"

	"github.com/BurntSushi/toml"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

// ============================================================================
// CONFIGURATION
// ============================================================================

// appConfig holds all application configuration.
var appConfig struct {
	deepgramAPIKey   string
	deepgramAgentURL string
	port             string
	host             string
	sessionSecret    []byte
}

// ============================================================================
// SESSION AUTH - JWT tokens for production security
// ============================================================================

// activeConnections tracks all active WebSocket connections for graceful shutdown.
var activeConnections sync.Map

// upgrader configures the WebSocket upgrade handler.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins in development
	},
}

const jwtExpiry = time.Hour

// issueToken creates a signed JWT with a 1-hour expiry.
func issueToken(secret []byte) (string, error) {
	claims := jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(jwtExpiry)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

// validateToken verifies a JWT token string and returns an error if invalid.
func validateToken(tokenStr string, secret []byte) error {
	_, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return secret, nil
	})
	return err
}

// validateWsToken extracts and validates a JWT from the access_token.<jwt> subprotocol.
// Returns the full subprotocol string if valid, empty string if invalid.
func validateWsToken(protocols []string, secret []byte) string {
	for _, proto := range protocols {
		if strings.HasPrefix(proto, "access_token.") {
			tokenStr := strings.TrimPrefix(proto, "access_token.")
			if err := validateToken(tokenStr, secret); err == nil {
				return proto
			}
		}
	}
	return ""
}

// ============================================================================
// METADATA - deepgram.toml parser
// ============================================================================

// DeepgramToml represents the structure of deepgram.toml.
type DeepgramToml struct {
	Meta map[string]interface{} `toml:"meta"`
}

// ============================================================================
// HTTP HANDLERS
// ============================================================================

// handleSession issues a signed JWT session token.
func handleSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	token, err := issueToken(appConfig.sessionSecret)
	if err != nil {
		log.Printf("Failed to issue token: %v", err)
		http.Error(w, `{"error":"INTERNAL_SERVER_ERROR","message":"Failed to issue session token"}`, http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"token": token})
}

// handleHealth returns a simple health check response.
// GET /health
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleMetadata returns project metadata from deepgram.toml.
func handleMetadata(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	var cfg DeepgramToml
	if _, err := toml.DecodeFile("deepgram.toml", &cfg); err != nil {
		log.Printf("Error reading deepgram.toml: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error":   "INTERNAL_SERVER_ERROR",
			"message": "Failed to read metadata from deepgram.toml",
		})
		return
	}

	if cfg.Meta == nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error":   "INTERNAL_SERVER_ERROR",
			"message": "Missing [meta] section in deepgram.toml",
		})
		return
	}

	json.NewEncoder(w).Encode(cfg.Meta)
}

// ============================================================================
// WEBSOCKET PROXY HANDLER
// ============================================================================

// agentHandler implements the Deepgram SDK AgentMessageChan interface and relays
// Voice Agent events to the browser WebSocket: audio as binary frames and all
// JSON events (including any not explicitly modeled, via UnhandledEvent) as text
// frames, preserving the wire format the frontend already expects.
type agentHandler struct {
	conn *websocket.Conn
	mu   *sync.Mutex
	// teardown closes the browser connection so a Deepgram-side close/error
	// propagates to the client and unblocks the handler's ReadMessage pump.
	// Safe to call multiple times.
	teardown func()

	// done is closed exactly once (via Close) when the session ends. The SDK
	// never closes the event channels it sends on, so each relay goroutine
	// selects on done to exit cleanly instead of blocking on its channel
	// forever (which would leak ~16 goroutines per connection).
	done      chan struct{}
	closeOnce sync.Once

	binaryChan           chan *[]byte
	openChan             chan *agentmsg.OpenResponse
	welcomeChan          chan *agentmsg.WelcomeResponse
	conversationChan     chan *agentmsg.ConversationTextResponse
	userStartedChan      chan *agentmsg.UserStartedSpeakingResponse
	agentThinkingChan    chan *agentmsg.AgentThinkingResponse
	functionCallChan     chan *agentmsg.FunctionCallRequestResponse
	agentStartedChan     chan *agentmsg.AgentStartedSpeakingResponse
	agentAudioDoneChan   chan *agentmsg.AgentAudioDoneResponse
	closeChan            chan *agentmsg.CloseResponse
	errorChan            chan *agentmsg.ErrorResponse
	unhandledChan        chan *[]byte
	injectionRefusedChan chan *agentmsg.InjectionRefusedResponse
	keepAliveChan        chan *agentmsg.KeepAlive
	settingsAppliedChan  chan *agentmsg.SettingsAppliedResponse
}

func newAgentHandler(conn *websocket.Conn, mu *sync.Mutex, teardown func()) *agentHandler {
	h := &agentHandler{
		conn:                 conn,
		mu:                   mu,
		teardown:             teardown,
		done:                 make(chan struct{}),
		binaryChan:           make(chan *[]byte),
		openChan:             make(chan *agentmsg.OpenResponse),
		welcomeChan:          make(chan *agentmsg.WelcomeResponse),
		conversationChan:     make(chan *agentmsg.ConversationTextResponse),
		userStartedChan:      make(chan *agentmsg.UserStartedSpeakingResponse),
		agentThinkingChan:    make(chan *agentmsg.AgentThinkingResponse),
		functionCallChan:     make(chan *agentmsg.FunctionCallRequestResponse),
		agentStartedChan:     make(chan *agentmsg.AgentStartedSpeakingResponse),
		agentAudioDoneChan:   make(chan *agentmsg.AgentAudioDoneResponse),
		closeChan:            make(chan *agentmsg.CloseResponse),
		errorChan:            make(chan *agentmsg.ErrorResponse),
		unhandledChan:        make(chan *[]byte),
		injectionRefusedChan: make(chan *agentmsg.InjectionRefusedResponse),
		keepAliveChan:        make(chan *agentmsg.KeepAlive),
		settingsAppliedChan:  make(chan *agentmsg.SettingsAppliedResponse),
	}
	go h.run()
	return h
}

// sendJSON marshals a Deepgram event and writes it to the browser as a text frame.
func (h *agentHandler) sendJSON(v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("Failed to marshal agent event: %v", err)
		return
	}
	h.sendText(data)
}

func (h *agentHandler) sendText(data []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Printf("Failed to forward agent event to client: %v", err)
	}
}

func (h *agentHandler) GetBinary() []*chan *[]byte { return []*chan *[]byte{&h.binaryChan} }
func (h *agentHandler) GetOpen() []*chan *agentmsg.OpenResponse {
	return []*chan *agentmsg.OpenResponse{&h.openChan}
}
func (h *agentHandler) GetWelcome() []*chan *agentmsg.WelcomeResponse {
	return []*chan *agentmsg.WelcomeResponse{&h.welcomeChan}
}
func (h *agentHandler) GetConversationText() []*chan *agentmsg.ConversationTextResponse {
	return []*chan *agentmsg.ConversationTextResponse{&h.conversationChan}
}
func (h *agentHandler) GetUserStartedSpeaking() []*chan *agentmsg.UserStartedSpeakingResponse {
	return []*chan *agentmsg.UserStartedSpeakingResponse{&h.userStartedChan}
}
func (h *agentHandler) GetAgentThinking() []*chan *agentmsg.AgentThinkingResponse {
	return []*chan *agentmsg.AgentThinkingResponse{&h.agentThinkingChan}
}
func (h *agentHandler) GetFunctionCallRequest() []*chan *agentmsg.FunctionCallRequestResponse {
	return []*chan *agentmsg.FunctionCallRequestResponse{&h.functionCallChan}
}
func (h *agentHandler) GetAgentStartedSpeaking() []*chan *agentmsg.AgentStartedSpeakingResponse {
	return []*chan *agentmsg.AgentStartedSpeakingResponse{&h.agentStartedChan}
}
func (h *agentHandler) GetAgentAudioDone() []*chan *agentmsg.AgentAudioDoneResponse {
	return []*chan *agentmsg.AgentAudioDoneResponse{&h.agentAudioDoneChan}
}
func (h *agentHandler) GetClose() []*chan *agentmsg.CloseResponse {
	return []*chan *agentmsg.CloseResponse{&h.closeChan}
}
func (h *agentHandler) GetError() []*chan *agentmsg.ErrorResponse {
	return []*chan *agentmsg.ErrorResponse{&h.errorChan}
}
func (h *agentHandler) GetUnhandled() []*chan *[]byte { return []*chan *[]byte{&h.unhandledChan} }
func (h *agentHandler) GetInjectionRefused() []*chan *agentmsg.InjectionRefusedResponse {
	return []*chan *agentmsg.InjectionRefusedResponse{&h.injectionRefusedChan}
}
func (h *agentHandler) GetKeepAlive() []*chan *agentmsg.KeepAlive {
	return []*chan *agentmsg.KeepAlive{&h.keepAliveChan}
}
func (h *agentHandler) GetSettingsApplied() []*chan *agentmsg.SettingsAppliedResponse {
	return []*chan *agentmsg.SettingsAppliedResponse{&h.settingsAppliedChan}
}

// Close signals every relay goroutine to exit. Safe to call multiple times;
// invoked when the session ends (client disconnect or Deepgram-side teardown).
//
// Call this only after the SDK client has been stopped (see handleVoiceAgent's
// defer order). The SDK sends on unbuffered channels, so if an event lands in
// the window between done closing and the SDK's read loop exiting, that send
// has no reader and parks the SDK goroutine. Stopping the client first shuts
// the read loop down before the readers go away.
func (h *agentHandler) Close() {
	h.closeOnce.Do(func() {
		close(h.done)
	})
}

// run relays every Deepgram Agent event to the browser connection. Each loop
// selects on h.done so it exits when the session ends: the SDK never closes
// the channels it sends on, so a plain `for range ch` would block forever and
// leak the goroutine (~16 per connection).
func (h *agentHandler) run() {
	go func() {
		for {
			select {
			case <-h.done:
				return
			case br, ok := <-h.binaryChan:
				if !ok {
					return
				}
				h.mu.Lock()
				if err := h.conn.WriteMessage(websocket.BinaryMessage, *br); err != nil {
					log.Printf("Failed to forward agent audio to client: %v", err)
				}
				h.mu.Unlock()
			}
		}
	}()
	go func() {
		for {
			select {
			case <-h.done:
				return
			case _, ok := <-h.openChan:
				if !ok {
					return
				}
			}
		}
	}()
	go func() {
		for {
			select {
			case <-h.done:
				return
			case v, ok := <-h.welcomeChan:
				if !ok {
					return
				}
				h.sendJSON(v)
			}
		}
	}()
	go func() {
		for {
			select {
			case <-h.done:
				return
			case v, ok := <-h.conversationChan:
				if !ok {
					return
				}
				h.sendJSON(v)
			}
		}
	}()
	go func() {
		for {
			select {
			case <-h.done:
				return
			case v, ok := <-h.userStartedChan:
				if !ok {
					return
				}
				h.sendJSON(v)
			}
		}
	}()
	go func() {
		for {
			select {
			case <-h.done:
				return
			case v, ok := <-h.agentThinkingChan:
				if !ok {
					return
				}
				h.sendJSON(v)
			}
		}
	}()
	go func() {
		for {
			select {
			case <-h.done:
				return
			case v, ok := <-h.functionCallChan:
				if !ok {
					return
				}
				h.sendJSON(v)
			}
		}
	}()
	go func() {
		for {
			select {
			case <-h.done:
				return
			case v, ok := <-h.agentStartedChan:
				if !ok {
					return
				}
				h.sendJSON(v)
			}
		}
	}()
	go func() {
		for {
			select {
			case <-h.done:
				return
			case v, ok := <-h.agentAudioDoneChan:
				if !ok {
					return
				}
				h.sendJSON(v)
			}
		}
	}()
	go func() {
		// Deepgram closed the session: tear down the browser connection so the
		// handler's ReadMessage pump returns instead of blocking until the
		// browser happens to disconnect.
		for {
			select {
			case <-h.done:
				return
			case _, ok := <-h.closeChan:
				if !ok {
					return
				}
				h.teardown()
			}
		}
	}()
	go func() {
		// Forward the error to the browser, then end the session — an agent
		// error is terminal, and leaving the pump blocked would leak the
		// connection and hang the UI.
		//
		// The relay map is hand-built with lowercase keys: the SDK's
		// ErrorResponse (an alias for DeepgramError) has no json:"type" tag, so
		// marshaling it directly emits {"Type":"Error",...} (capital T) and the
		// frontend's `case 'Error'` never fires. Emit type/description/code the
		// way the browser expects.
		//
		// The code is a fixed PROVIDER_ERROR rather than the SDK's ErrCode.
		// Deepgram's wire Error event is {type, description, code}, but the SDK
		// struct maps its code field to `err_code`, so the wire code is dropped
		// during unmarshal and ErrCode is always empty here; the errors the SDK
		// synthesizes from a transport failure never set it either. The starter
		// error contract restricts code to its own enum in any case, and
		// PROVIDER_ERROR is the value it reserves for an upstream failure —
		// matching what the other voice-agent starters relay.
		for {
			select {
			case <-h.done:
				return
			case v, ok := <-h.errorChan:
				if !ok {
					return
				}
				h.sendJSON(map[string]any{
					"type":        "Error",
					"description": v.Description,
					"code":        "PROVIDER_ERROR",
				})
				h.teardown()
			}
		}
	}()
	go func() {
		// Events not explicitly modeled by the SDK (e.g. PromptUpdated, SpeakUpdated,
		// FunctionCallResponse) arrive here as raw bytes; forward them verbatim.
		for {
			select {
			case <-h.done:
				return
			case br, ok := <-h.unhandledChan:
				if !ok {
					return
				}
				h.sendText(*br)
			}
		}
	}()
	go func() {
		for {
			select {
			case <-h.done:
				return
			case v, ok := <-h.injectionRefusedChan:
				if !ok {
					return
				}
				h.sendJSON(v)
			}
		}
	}()
	go func() {
		for {
			select {
			case <-h.done:
				return
			case _, ok := <-h.keepAliveChan:
				if !ok {
					return
				}
			}
		}
	}()
	go func() {
		for {
			select {
			case <-h.done:
				return
			case v, ok := <-h.settingsAppliedChan:
				if !ok {
					return
				}
				h.sendJSON(v)
			}
		}
	}()
}

// sendClientError writes a contract-shaped error frame to the browser.
func sendClientError(conn *websocket.Conn, mu *sync.Mutex, code, description string) {
	msg, _ := json.Marshal(map[string]string{
		"type":        "Error",
		"description": description,
		"code":        code,
	})
	mu.Lock()
	defer mu.Unlock()
	conn.WriteMessage(websocket.TextMessage, msg)
}

// handleVoiceAgent bridges the browser WebSocket to Deepgram's Voice Agent API
// using the official Go SDK. The frontend still sends its own Settings message
// first; it is parsed into the SDK SettingsOptions and sent to Deepgram on connect.
func handleVoiceAgent(w http.ResponseWriter, r *http.Request) {
	// Validate JWT from access_token.<jwt> subprotocol
	protocols := websocket.Subprotocols(r)
	validProto := validateWsToken(protocols, appConfig.sessionSecret)
	if validProto == "" {
		log.Println("WebSocket auth failed: invalid or missing token")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Upgrade with the accepted subprotocol echoed back
	responseHeader := http.Header{}
	responseHeader.Set("Sec-WebSocket-Protocol", validProto)

	clientConn, err := upgrader.Upgrade(w, r, responseHeader)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	log.Println("Client connected to /api/voice-agent")
	activeConnections.Store(clientConn, true)
	defer func() {
		activeConnections.Delete(clientConn)
		clientConn.Close()
	}()

	// Serialize all writes to the browser connection (handler goroutines + close frames).
	writeMu := &sync.Mutex{}

	// The frontend sends a Settings message first. Parse it into the SDK's
	// SettingsOptions so the SDK sends the Deepgram-formatted Settings on connect.
	settings := agent.NewSettingsConfigurationOptions()
	for {
		msgType, data, err := clientConn.ReadMessage()
		if err != nil {
			log.Printf("Client disconnected before sending Settings: %v", err)
			return
		}
		if msgType != websocket.TextMessage {
			continue
		}
		if err := json.Unmarshal(data, settings); err != nil {
			log.Printf("Failed to parse Settings from client: %v", err)
			sendClientError(clientConn, writeMu, "INVALID_SETTINGS", "Invalid Settings message")
			return
		}
		break
	}

	log.Println("Initiating Deepgram Agent connection...")
	cOptions := &dginterfaces.ClientOptions{EnableKeepAlive: true}

	// teardown tears down the browser session exactly once: send a close frame
	// and close the connection (which unblocks the ReadMessage pump below). It
	// is invoked by the SDK handler goroutines on a Deepgram-side close/error.
	var closeOnce sync.Once
	teardown := func() {
		closeOnce.Do(func() {
			writeMu.Lock()
			clientConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			writeMu.Unlock()
			clientConn.Close()
		})
	}
	handler := newAgentHandler(clientConn, writeMu, teardown)
	// Signal the relay goroutines to exit when this handler returns (client
	// disconnect or any error path), so they don't leak.
	//
	// Registered BEFORE the dgClient.Stop() defer below, so it runs AFTER it —
	// the ordering is load-bearing. Stop() pushes a CloseResponse onto the
	// handler's channel synchronously while holding the SDK's connection mutex,
	// so if the relay goroutines were already gone that send would block
	// forever and hang this request goroutine, not merely leak one.
	defer handler.Close()

	dgClient, err := agent.NewWSUsingChan(context.Background(), appConfig.deepgramAPIKey, cOptions, settings, agentmsg.AgentMessageChan(handler))
	if err != nil {
		log.Printf("Failed to create Deepgram Agent client: %v", err)
		sendClientError(clientConn, writeMu, "CONNECTION_FAILED", "Failed to establish proxy connection")
		return
	}

	if !dgClient.Connect() {
		log.Printf("Deepgram Agent connection failed")
		sendClientError(clientConn, writeMu, "CONNECTION_FAILED", "Failed to establish proxy connection")
		return
	}
	// Must run before handler.Close() — see the note on that defer above.
	defer dgClient.Stop()

	log.Println("Connected to Deepgram Agent API")

	// Pump audio (binary) and control (text) messages from the browser to Deepgram.
	for {
		msgType, data, err := clientConn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("Client read error: %v", err)
			} else {
				log.Println("Client disconnected")
			}
			break
		}

		switch msgType {
		case websocket.BinaryMessage:
			if err := dgClient.WriteBinary(data); err != nil {
				log.Printf("Error writing audio to Deepgram: %v", err)
				return
			}
		case websocket.TextMessage:
			// Control messages (UpdateSpeak, UpdatePrompt, InjectAgentMessage, ...)
			var msg map[string]interface{}
			if err := json.Unmarshal(data, &msg); err != nil {
				log.Printf("Ignoring non-JSON control message: %v", err)
				continue
			}
			if err := dgClient.WriteJSON(msg); err != nil {
				log.Printf("Error forwarding control message to Deepgram: %v", err)
			}
		}
	}

	log.Println("Voice agent session ending")
	writeMu.Lock()
	clientConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	writeMu.Unlock()
}

// ============================================================================
// GRACEFUL SHUTDOWN
// ============================================================================

// gracefulShutdown closes all active connections and stops the server.
func gracefulShutdown(server *http.Server, sig string) {
	log.Printf("\n%s signal received: starting graceful shutdown...", sig)

	// Close all active WebSocket connections
	count := 0
	activeConnections.Range(func(key, value interface{}) bool {
		conn := key.(*websocket.Conn)
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseGoingAway, "Server shutting down"))
		conn.Close()
		count++
		return true
	})
	log.Printf("Closed %d active WebSocket connection(s)", count)

	// Shutdown HTTP server with a 10-second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	log.Println("Shutdown complete")
}

// ============================================================================
// MAIN
// ============================================================================

func main() {
	// Load configuration from environment variables
	appConfig.deepgramAPIKey = os.Getenv("DEEPGRAM_API_KEY")
	if appConfig.deepgramAPIKey == "" {
		log.Fatal("ERROR: DEEPGRAM_API_KEY environment variable is required\n" +
			"Please copy sample.env to .env and add your API key")
	}

	// Voice Agent uses agent.deepgram.com, not api.deepgram.com
	appConfig.deepgramAgentURL = "wss://agent.deepgram.com/v1/agent/converse"

	appConfig.port = os.Getenv("PORT")
	if appConfig.port == "" {
		appConfig.port = "8081"
	}

	appConfig.host = os.Getenv("HOST")
	if appConfig.host == "" {
		appConfig.host = "0.0.0.0"
	}

	secret := os.Getenv("SESSION_SECRET")
	if secret != "" {
		appConfig.sessionSecret = []byte(secret)
	} else {
		appConfig.sessionSecret = make([]byte, 32)
		if _, err := rand.Read(appConfig.sessionSecret); err != nil {
			log.Fatal("Failed to generate session secret:", err)
		}
	}

	// Initialize the Deepgram Go SDK.
	agent.InitWithDefault()

	// Register HTTP and WebSocket routes
	mux := http.NewServeMux()
	mux.HandleFunc("/api/session", handleSession)
	mux.HandleFunc("/api/metadata", handleMetadata)
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/api/voice-agent", handleVoiceAgent)

	addr := fmt.Sprintf("%s:%s", appConfig.host, appConfig.port)
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		sig := <-sigChan
		gracefulShutdown(server, sig.String())
		os.Exit(0)
	}()

	// Start server
	log.Println(strings.Repeat("=", 70))
	log.Printf("Backend API Server running at http://localhost:%s", appConfig.port)
	log.Println("")
	log.Println("GET  /api/session")
	log.Println("WS   /api/voice-agent (auth required)")
	log.Println("GET  /api/metadata")
	log.Println("GET  /health")
	log.Println(strings.Repeat("=", 70))

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}
