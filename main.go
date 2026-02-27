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

// reservedCloseCodes lists WebSocket close codes that cannot be set by applications.
// Per RFC 6455, codes 1004, 1005, 1006, and 1015 are reserved.
var reservedCloseCodes = map[int]bool{
	1004: true,
	1005: true,
	1006: true,
	1015: true,
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
// WEBSOCKET HELPERS
// ============================================================================

// getSafeCloseCode returns a valid WebSocket close code.
// Reserved codes (1004, 1005, 1006, 1015) are translated to 1000 (normal closure).
func getSafeCloseCode(code int) int {
	if code >= 1000 && code <= 4999 && !reservedCloseCodes[code] {
		return code
	}
	return websocket.CloseNormalClosure
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

// handleVoiceAgent proxies WebSocket connections to Deepgram's Voice Agent API.
// It forwards all messages (JSON and binary) bidirectionally without modification.
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

	// Connect to Deepgram Voice Agent API
	// No query parameters needed -- config is sent via JSON after connection
	log.Println("Initiating Deepgram connection...")
	deepgramHeader := http.Header{}
	deepgramHeader.Set("Authorization", fmt.Sprintf("Token %s", appConfig.deepgramAPIKey))

	deepgramConn, _, err := websocket.DefaultDialer.Dial(appConfig.deepgramAgentURL, deepgramHeader)
	if err != nil {
		log.Printf("Failed to connect to Deepgram: %v", err)
		errMsg, _ := json.Marshal(map[string]string{
			"type":        "Error",
			"description": "Failed to establish proxy connection",
			"code":        "CONNECTION_FAILED",
		})
		clientConn.WriteMessage(websocket.TextMessage, errMsg)
		clientConn.Close()
		activeConnections.Delete(clientConn)
		return
	}

	log.Println("Connected to Deepgram Agent API")

	// done channels signal when each forwarding goroutine finishes
	clientDone := make(chan struct{})
	deepgramDone := make(chan struct{})

	// Forward messages: Deepgram -> Client
	go func() {
		defer close(deepgramDone)
		for {
			messageType, data, err := deepgramConn.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					log.Println("Deepgram connection closed normally")
				} else {
					log.Printf("Deepgram read error: %v", err)
				}
				// Translate reserved close codes to 1000 before forwarding to client
				closeCode := websocket.CloseNormalClosure
				if ce, ok := err.(*websocket.CloseError); ok {
					closeCode = getSafeCloseCode(ce.Code)
				}
				clientConn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(closeCode, ""))
				return
			}
			if err := clientConn.WriteMessage(messageType, data); err != nil {
				log.Printf("Error forwarding to client: %v", err)
				return
			}
		}
	}()

	// Forward messages: Client -> Deepgram
	go func() {
		defer close(clientDone)
		for {
			messageType, data, err := clientConn.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					log.Println("Client disconnected normally")
				} else {
					log.Printf("Client read error: %v", err)
				}
				return
			}
			if err := deepgramConn.WriteMessage(messageType, data); err != nil {
				log.Printf("Error forwarding to Deepgram: %v", err)
				return
			}
		}
	}()

	// Wait for either side to close, then clean up both
	select {
	case <-clientDone:
		log.Println("Client disconnected, closing Deepgram connection")
		deepgramConn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Client disconnected"))
		deepgramConn.Close()
	case <-deepgramDone:
		log.Println("Deepgram disconnected, closing client connection")
		clientConn.Close()
	}

	activeConnections.Delete(clientConn)
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
