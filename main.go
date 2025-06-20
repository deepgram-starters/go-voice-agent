package main

// streaming
import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	msginterfaces "github.com/deepgram/deepgram-go-sdk/v3/pkg/api/agent/v1/websocket/interfaces"
	microphone "github.com/deepgram/deepgram-go-sdk/v3/pkg/audio/microphone"
	client "github.com/deepgram/deepgram-go-sdk/v3/pkg/client/agent"
	interfaces "github.com/deepgram/deepgram-go-sdk/v3/pkg/client/interfaces"
	"github.com/gorilla/websocket"
)

// WebSocket upgrader
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for development
	},
}

// WebSocket connection manager
type WebSocketManager struct {
	connections map[*websocket.Conn]bool
	mutex       sync.RWMutex
}

func NewWebSocketManager() *WebSocketManager {
	return &WebSocketManager{
		connections: make(map[*websocket.Conn]bool),
	}
}

func (wm *WebSocketManager) AddConnection(conn *websocket.Conn) {
	wm.mutex.Lock()
	defer wm.mutex.Unlock()
	wm.connections[conn] = true
}

func (wm *WebSocketManager) RemoveConnection(conn *websocket.Conn) {
	wm.mutex.Lock()
	defer wm.mutex.Unlock()
	delete(wm.connections, conn)
}

func (wm *WebSocketManager) Broadcast(message interface{}) {
	wm.mutex.RLock()
	defer wm.mutex.RUnlock()

	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("Error marshaling message: %v", err)
		return
	}

	for conn := range wm.connections {
		err := conn.WriteMessage(websocket.TextMessage, data)
		if err != nil {
			log.Printf("Error sending message: %v", err)
			conn.Close()
			delete(wm.connections, conn)
		}
	}
}

// MyHandler implements the message handler interface for Deepgram Voice Agent
type MyHandler struct {
	binaryChan                   chan *[]byte
	openChan                     chan *msginterfaces.OpenResponse
	welcomeResponse              chan *msginterfaces.WelcomeResponse
	conversationTextResponse     chan *msginterfaces.ConversationTextResponse
	userStartedSpeakingResponse  chan *msginterfaces.UserStartedSpeakingResponse
	agentThinkingResponse        chan *msginterfaces.AgentThinkingResponse
	functionCallRequestResponse  chan *msginterfaces.FunctionCallRequestResponse
	agentStartedSpeakingResponse chan *msginterfaces.AgentStartedSpeakingResponse
	agentAudioDoneResponse       chan *msginterfaces.AgentAudioDoneResponse
	closeChan                    chan *msginterfaces.CloseResponse
	errorChan                    chan *msginterfaces.ErrorResponse
	unhandledChan                chan *[]byte
	injectionRefusedResponse     chan *msginterfaces.InjectionRefusedResponse
	keepAliveResponse            chan *msginterfaces.KeepAlive
	settingsAppliedResponse      chan *msginterfaces.SettingsAppliedResponse
	wsManager                    *WebSocketManager
}

// NewMyHandler creates and initializes a new message handler
func NewMyHandler(wsManager *WebSocketManager) *MyHandler {
	handler := &MyHandler{
		binaryChan:                   make(chan *[]byte),
		openChan:                     make(chan *msginterfaces.OpenResponse),
		welcomeResponse:              make(chan *msginterfaces.WelcomeResponse),
		conversationTextResponse:     make(chan *msginterfaces.ConversationTextResponse),
		userStartedSpeakingResponse:  make(chan *msginterfaces.UserStartedSpeakingResponse),
		agentThinkingResponse:        make(chan *msginterfaces.AgentThinkingResponse),
		functionCallRequestResponse:  make(chan *msginterfaces.FunctionCallRequestResponse),
		agentStartedSpeakingResponse: make(chan *msginterfaces.AgentStartedSpeakingResponse),
		agentAudioDoneResponse:       make(chan *msginterfaces.AgentAudioDoneResponse),
		closeChan:                    make(chan *msginterfaces.CloseResponse),
		errorChan:                    make(chan *msginterfaces.ErrorResponse),
		unhandledChan:                make(chan *[]byte),
		injectionRefusedResponse:     make(chan *msginterfaces.InjectionRefusedResponse),
		keepAliveResponse:            make(chan *msginterfaces.KeepAlive),
		settingsAppliedResponse:      make(chan *msginterfaces.SettingsAppliedResponse),
		wsManager:                    wsManager,
	}

	go func() {
		handler.Run()
	}()

	return handler
}

// GetBinary returns the binary channels
func (dch MyHandler) GetBinary() []*chan *[]byte {
	return []*chan *[]byte{&dch.binaryChan}
}

// GetOpen returns the open channels
func (dch MyHandler) GetOpen() []*chan *msginterfaces.OpenResponse {
	return []*chan *msginterfaces.OpenResponse{&dch.openChan}
}

// GetWelcomeResponse returns the welcome response channels
func (dch MyHandler) GetWelcome() []*chan *msginterfaces.WelcomeResponse {
	return []*chan *msginterfaces.WelcomeResponse{&dch.welcomeResponse}
}

// GetConversationTextResponse returns the conversation text response channels
func (dch MyHandler) GetConversationText() []*chan *msginterfaces.ConversationTextResponse {
	return []*chan *msginterfaces.ConversationTextResponse{&dch.conversationTextResponse}
}

// GetUserStartedSpeakingResponse returns the user started speaking response channels
func (dch MyHandler) GetUserStartedSpeaking() []*chan *msginterfaces.UserStartedSpeakingResponse {
	return []*chan *msginterfaces.UserStartedSpeakingResponse{&dch.userStartedSpeakingResponse}
}

// GetAgentThinkingResponse returns the agent thinking response channels
func (dch MyHandler) GetAgentThinking() []*chan *msginterfaces.AgentThinkingResponse {
	return []*chan *msginterfaces.AgentThinkingResponse{&dch.agentThinkingResponse}
}

// GetFunctionCallRequestResponse returns the function call request response channels
func (dch MyHandler) GetFunctionCallRequest() []*chan *msginterfaces.FunctionCallRequestResponse {
	return []*chan *msginterfaces.FunctionCallRequestResponse{&dch.functionCallRequestResponse}
}

// GetAgentStartedSpeakingResponse returns the agent started speaking response channels
func (dch MyHandler) GetAgentStartedSpeaking() []*chan *msginterfaces.AgentStartedSpeakingResponse {
	return []*chan *msginterfaces.AgentStartedSpeakingResponse{&dch.agentStartedSpeakingResponse}
}

// GetAgentAudioDoneResponse returns the agent audio done response channels
func (dch MyHandler) GetAgentAudioDone() []*chan *msginterfaces.AgentAudioDoneResponse {
	return []*chan *msginterfaces.AgentAudioDoneResponse{&dch.agentAudioDoneResponse}
}

// GetClose returns the close channels
func (dch MyHandler) GetClose() []*chan *msginterfaces.CloseResponse {
	return []*chan *msginterfaces.CloseResponse{&dch.closeChan}
}

// GetError returns the error channels
func (dch MyHandler) GetError() []*chan *msginterfaces.ErrorResponse {
	return []*chan *msginterfaces.ErrorResponse{&dch.errorChan}
}

// GetUnhandled returns the unhandled event channels
func (dch MyHandler) GetUnhandled() []*chan *[]byte {
	return []*chan *[]byte{&dch.unhandledChan}
}

// GetInjectionRefused returns the injection refused response channels
func (dch MyHandler) GetInjectionRefused() []*chan *msginterfaces.InjectionRefusedResponse {
	return []*chan *msginterfaces.InjectionRefusedResponse{&dch.injectionRefusedResponse}
}

// GetKeepAlive returns the keep alive channels
func (dch MyHandler) GetKeepAlive() []*chan *msginterfaces.KeepAlive {
	return []*chan *msginterfaces.KeepAlive{&dch.keepAliveResponse}
}

// GetSettingsApplied returns the settings applied response channels
func (dch MyHandler) GetSettingsApplied() []*chan *msginterfaces.SettingsAppliedResponse {
	return []*chan *msginterfaces.SettingsAppliedResponse{&dch.settingsAppliedResponse}
}

// Run handles all incoming messages from the Deepgram Voice Agent API
// This function processes all the different message types and prints them to the terminal
func (dch MyHandler) Run() error {
	wgReceivers := sync.WaitGroup{}

	// binary channel - handles audio data from the agent
	wgReceivers.Add(1)
	go func() {
		defer wgReceivers.Done()

		counter := 0
		lastBytesReceived := time.Now().Add(-7 * time.Second)

		for br := range dch.binaryChan {
			fmt.Printf("\n\n[Binary Data Received]\n")
			fmt.Printf("Size: %d bytes\n", len(*br))

			// Broadcast audio data to WebSocket clients
			if dch.wsManager != nil {
				dch.wsManager.Broadcast(map[string]interface{}{
					"type":  "agent_speaking",
					"audio": *br,
				})
			}

			if lastBytesReceived.Add(5 * time.Second).Before(time.Now()) {
				counter = counter + 1
				file, err := os.OpenFile(fmt.Sprintf("output_%d.wav", counter), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o666)
				if err != nil {
					fmt.Printf("Failed to open file. Err: %v\n", err)
					continue
				}
				// Add a wav audio container header to the file if you want to play the audio
				// using a media player like VLC, Media Player, or Apple Music
				header := []byte{
					0x52, 0x49, 0x46, 0x46, // "RIFF"
					0x00, 0x00, 0x00, 0x00, // Placeholder for file size
					0x57, 0x41, 0x56, 0x45, // "WAVE"
					0x66, 0x6d, 0x74, 0x20, // "fmt "
					0x10, 0x00, 0x00, 0x00, // Chunk size (16)
					0x01, 0x00, // Audio format (1 for PCM)
					0x01, 0x00, // Number of channels (1)
					0x80, 0x3e, 0x00, 0x00, // Sample rate (16000)
					0x00, 0x7d, 0x00, 0x00, // Byte rate (16000 * 2)
					0x02, 0x00, // Block align (2)
					0x10, 0x00, // Bits per sample (16)
					0x64, 0x61, 0x74, 0x61, // "data"
					0x00, 0x00, 0x00, 0x00, // Placeholder for data size
				}

				_, err = file.Write(header)
				if err != nil {
					fmt.Printf("Failed to write header to file. Err: %v\n", err)
					continue
				}
				file.Close()
			}

			fmt.Printf("Dumping to WAV file\n")
			file, err := os.OpenFile(fmt.Sprintf("output_%d.wav", counter), os.O_APPEND|os.O_WRONLY, 0o644)
			if err != nil {
				fmt.Printf("Failed to open file. Err: %v\n", err)
				continue
			}

			_, err = file.Write(*br)
			file.Close()

			if err != nil {
				fmt.Printf("Failed to write to file. Err: %v\n", err)
				continue
			}

			lastBytesReceived = time.Now()
		}
	}()

	// open channel - handles connection open events
	wgReceivers.Add(1)
	go func() {
		defer wgReceivers.Done()

		for _ = range dch.openChan {
			fmt.Printf("\n\n[OpenResponse]\n\n")
		}
	}()

	// welcome response channel - handles agent welcome messages
	wgReceivers.Add(1)
	go func() {
		defer wgReceivers.Done()

		for _ = range dch.welcomeResponse {
			fmt.Printf("\n\n[WelcomeResponse]\n\n")
		}
	}()

	// conversation text response channel - handles text conversation messages
	wgReceivers.Add(1)
	go func() {
		defer wgReceivers.Done()

		for ctr := range dch.conversationTextResponse {
			fmt.Printf("\n\n[ConversationTextResponse]\n")
			fmt.Printf("%s: %s\n\n", ctr.Role, ctr.Content)

			// Broadcast conversation text to WebSocket clients
			if dch.wsManager != nil {
				dch.wsManager.Broadcast(map[string]interface{}{
					"type":    "conversation_text",
					"role":    ctr.Role,
					"content": ctr.Content,
				})
			}
		}
	}()

	// user started speaking response channel - handles user speech start events
	wgReceivers.Add(1)
	go func() {
		defer wgReceivers.Done()

		for _ = range dch.userStartedSpeakingResponse {
			fmt.Printf("\n\n[UserStartedSpeakingResponse]\n\n")
		}
	}()

	// agent thinking response channel - handles agent thinking events
	wgReceivers.Add(1)
	go func() {
		defer wgReceivers.Done()

		for _ = range dch.agentThinkingResponse {
			fmt.Printf("\n\n[AgentThinkingResponse]\n\n")
		}
	}()

	// function call request response channel - handles function call requests
	wgReceivers.Add(1)
	go func() {
		defer wgReceivers.Done()

		for _ = range dch.functionCallRequestResponse {
			fmt.Printf("\n\n[FunctionCallRequestResponse]\n\n")
		}
	}()

	// agent started speaking response channel - handles agent speech start events
	wgReceivers.Add(1)
	go func() {
		defer wgReceivers.Done()

		for _ = range dch.agentStartedSpeakingResponse {
			fmt.Printf("\n\n[AgentStartedSpeakingResponse]\n\n")
		}
	}()

	// agent audio done response channel - handles agent speech end events
	wgReceivers.Add(1)
	go func() {
		defer wgReceivers.Done()

		for _ = range dch.agentAudioDoneResponse {
			fmt.Printf("\n\n[AgentAudioDoneResponse]\n\n")
		}
	}()

	// keep alive response channel - handles keep alive messages
	wgReceivers.Add(1)
	go func() {
		defer wgReceivers.Done()

		for _ = range dch.keepAliveResponse {
			fmt.Printf("\n\n[KeepAliveResponse]\n\n")
		}
	}()

	// settings applied response channel - handles settings confirmation
	wgReceivers.Add(1)
	go func() {
		defer wgReceivers.Done()

		for _ = range dch.settingsAppliedResponse {
			fmt.Printf("\n\n[SettingsAppliedResponse]\n\n")
		}
	}()

	// close channel - handles connection close events
	wgReceivers.Add(1)
	go func() {
		defer wgReceivers.Done()

		for _ = range dch.closeChan {
			fmt.Printf("\n\n[CloseResponse]\n\n")
		}
	}()

	// error channel - handles error messages
	wgReceivers.Add(1)
	go func() {
		defer wgReceivers.Done()

		for er := range dch.errorChan {
			fmt.Printf("\n[ErrorResponse]\n")
			fmt.Printf("\nError.Type: %s\n", er.ErrCode)
			fmt.Printf("Error.Message: %s\n", er.ErrMsg)
			fmt.Printf("Error.Description: %s\n\n", er.Description)
			fmt.Printf("Error.Variant: %s\n\n", er.Variant)
		}
	}()

	// unhandled event channel - handles any unhandled message types
	wgReceivers.Add(1)
	go func() {
		defer wgReceivers.Done()

		for byData := range dch.unhandledChan {
			fmt.Printf("\n[UnhandledEvent]\n")
			fmt.Printf("Raw message: %s\n", string(*byData))
		}
	}()

	// wait for all receivers to finish
	wgReceivers.Wait()

	return nil
}

// serveWebPage serves the HTML page for browser microphone access
func serveWebPage(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("templates/index.html")
	if err != nil {
		http.Error(w, "Error loading template", http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, nil)
	if err != nil {
		http.Error(w, "Error executing template", http.StatusInternalServerError)
		return
	}
}

// handleWebSocket handles WebSocket connections for the voice agent interface
func handleWebSocket(wsManager *WebSocketManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("Error upgrading connection: %v", err)
			return
		}

		wsManager.AddConnection(conn)
		log.Printf("New WebSocket connection established")

		// Send initial connection message
		conn.WriteJSON(map[string]interface{}{
			"type":    "connected",
			"message": "Connected to Voice Agent",
		})

		// Handle incoming messages
		for {
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				log.Printf("Error reading message: %v", err)
				wsManager.RemoveConnection(conn)
				conn.Close()
				break
			}

			// Handle binary audio data
			if messageType == websocket.BinaryMessage {
				log.Printf("Received binary audio data: %d bytes", len(message))
				// Here you would forward the audio data to the Deepgram Voice Agent
				// For now, we'll just log it
			}

			// Handle text messages
			if messageType == websocket.TextMessage {
				log.Printf("Received text message: %s", string(message))
			}
		}
	}
}

func main() {
	// Check for required environment variable
	apiKey := os.Getenv("DEEPGRAM_API_KEY")
	if apiKey == "" {
		fmt.Println("ERROR: DEEPGRAM_API_KEY environment variable is required")
		fmt.Println("Please set it with: export DEEPGRAM_API_KEY=\"YOUR_DEEPGRAM_API_KEY\"")
		os.Exit(1)
	}

	// Create WebSocket manager
	wsManager := NewWebSocketManager()

	// Start web server for browser access
	go func() {
		http.HandleFunc("/", serveWebPage)
		http.HandleFunc("/socket.io/", handleWebSocket(wsManager))

		fmt.Println("Starting web server on http://localhost:3000")
		fmt.Println("Open your browser and navigate to http://localhost:3000 to access the voice agent interface")
		log.Fatal(http.ListenAndServe(":3000", nil))
	}()

	// init library
	microphone.Initialize()

	// print instructions
	fmt.Print("\n\nPress ENTER to exit!\n\n")

	/*
		DG Streaming API
	*/
	// init library
	client.Init(client.InitLib{
		LogLevel: client.LogLevelDefault, // LogLevelDefault, LogLevelFull, LogLevelDebug, LogLevelTrace
	})

	// Go context
	ctx := context.Background()
	// client options
	cOptions := &interfaces.ClientOptions{
		EnableKeepAlive: true,
	}

	// set the Transcription options
	tOptions := client.NewSettingsConfigurationOptions()
	tOptions.Agent.Think.Provider["type"] = "open_ai"
	tOptions.Agent.Think.Provider["model"] = "gpt-4o-mini"
	tOptions.Agent.Think.Prompt = "You are a helpful AI assistant."
	tOptions.Agent.Listen.Provider["type"] = "deepgram"
	tOptions.Agent.Listen.Provider["model"] = "nova-3"
	tOptions.Agent.Listen.Provider["keyterms"] = []string{"Bueller"}
	tOptions.Agent.Language = "en"
	tOptions.Agent.Greeting = "Hello! How can I help you today?"

	// implement your own callback
	callback := msginterfaces.AgentMessageChan(*NewMyHandler(wsManager))

	// create a Deepgram client
	fmt.Printf("Creating new Deepgram WebSocket client...\n")
	dgClient, err := client.NewWSUsingChan(ctx, apiKey, cOptions, tOptions, callback)
	if err != nil {
		fmt.Printf("ERROR creating LiveTranscription connection:\n- Error: %v\n- Type: %T\n", err, err)
		return
	}

	// connect the websocket to Deepgram
	fmt.Printf("Attempting to connect to Deepgram WebSocket...\n")
	bConnected := dgClient.Connect()
	if !bConnected {
		fmt.Printf("WebSocket connection failed - check your API key and network connection\n")
		os.Exit(1)
	}
	fmt.Printf("Successfully connected to Deepgram WebSocket\n")

	/*
		Microphone package
	*/
	// mic stuff
	fmt.Printf("Initializing microphone...\n")
	mic, err := microphone.New(microphone.AudioConfig{
		InputChannels: 1,
		SamplingRate:  16000,
	})
	if err != nil {
		fmt.Printf("Initialize failed. Err: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Microphone initialized successfully\n")

	// start the mic
	fmt.Printf("Starting Microphone...\n")
	err = mic.Start()
	if err != nil {
		fmt.Printf("mic.Start failed. Err: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Microphone started successfully\n")

	go func() {
		fmt.Printf("Starting audio stream...\n")
		// feed the microphone stream to the Deepgram client (this is a blocking call)
		mic.Stream(dgClient)
		fmt.Printf("Audio stream ended\n")
	}()

	// wait for user input to exit
	input := bufio.NewScanner(os.Stdin)
	input.Scan()

	// close mic stream
	fmt.Printf("Stopping Microphone...\n")
	err = mic.Stop()
	if err != nil {
		fmt.Printf("mic.Stop failed. Err: %v\n", err)
		os.Exit(1)
	}

	// teardown library
	microphone.Teardown()

	// close DG client
	fmt.Printf("Stopping Agent...\n")
	dgClient.Stop()

	fmt.Printf("\n\nProgram exiting...\n")
}
