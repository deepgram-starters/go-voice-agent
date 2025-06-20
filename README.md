# Go Voice Agent Starter

Get started using Deepgram's Voice Agent with this Go demo app. This starter application demonstrates how to build a voice agent that can listen to your microphone, process speech, and respond with AI-generated audio.

## Description

This Go application showcases Deepgram's Voice Agent API capabilities. It includes:

- **Terminal Interface**: Run the app in your terminal to interact with the voice agent using your computer's microphone
- **Web Interface**: Launch a web server that provides a browser-based microphone interface
- **Real-time Processing**: Stream audio to Deepgram's Voice Agent API for live conversation
- **Audio Output**: Save agent responses as WAV files for playback

The app demonstrates how to handle all Voice Agent message types including conversation text, user speech events, agent thinking, and binary audio data.

## Prerequisites

Before running this application, you'll need:

- **Go 1.21 or higher** installed on your system
- **A Deepgram API Key** - Get one for free at [console.deepgram.com](https://console.deepgram.com)
- **Microphone access** on your device
- **Internet connection** for API communication

## Getting Started

### 1. Clone and Setup

```bash
git clone <repository-url>
cd go-voice-agent
go mod tidy
```

### 2. Set Your API Key

Set your Deepgram API key as an environment variable:

```bash
export DEEPGRAM_API_KEY="YOUR_DEEPGRAM_API_KEY"
```

### 3. Run the Application

Start the application with:

```bash
go run main.go
```

The app will:
- Start a web server on `http://localhost:3000`
- Initialize the microphone for terminal use
- Connect to Deepgram's Voice Agent API
- Display all interactions in the terminal

### 4. Use the Application

#### Terminal Mode
- Speak into your microphone when prompted
- View real-time conversation text in the terminal
- Agent responses are saved as WAV files (`output_1.wav`, `output_2.wav`, etc.)
- Press Enter to exit

#### Web Mode
- Open your browser and navigate to `http://localhost:3000`
- The page will automatically request microphone access
- Audio will be streamed to the voice agent in real-time
- Agent responses will be played back through your speakers

## How It Works

The application uses Deepgram's Go SDK to:

1. **Initialize** the Voice Agent with OpenAI GPT-4o-mini for thinking and Deepgram Nova-3 for speech recognition
2. **Stream** microphone audio to the Voice Agent API in real-time
3. **Process** all message types (conversation text, speech events, thinking, etc.)
4. **Save** agent audio responses as WAV files
5. **Display** all interactions in the terminal for debugging

## Configuration

The agent is configured with:
- **Thinking Provider**: OpenAI GPT-4o-mini
- **Listening Provider**: Deepgram Nova-3
- **Language**: English
- **Greeting**: "Hello! How can I help you today?"

You can modify these settings in the `main.go` file under the `tOptions` configuration section.

## Troubleshooting

### Common Issues

- **"DEEPGRAM_API_KEY environment variable is required"**: Make sure you've set your API key correctly
- **"WebSocket connection failed"**: Check your internet connection and API key validity
- **"Microphone access failed"**: Ensure your browser/terminal has microphone permissions

### Getting Help

If you encounter any issues:

1. Check the [Deepgram Documentation](https://developers.deepgram.com)
2. Join our [Discord Community](https://discord.gg/deepgram) for real-time support
3. Search existing [GitHub Issues](https://github.com/deepgram/starter-apps/issues) for similar problems

## Contributing

We welcome contributions! Please see our [Contributing Guidelines](CONTRIBUTING.md) for details on how to submit pull requests, report bugs, or suggest new features.

## Security

For security-related questions or to report vulnerabilities, please review our [Security Policy](SECURITY.md).

## Code of Conduct

This project follows our [Code of Conduct](CODE_OF_CONDUCT.md). Please read it to understand our community standards.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Support

- **Documentation**: [developers.deepgram.com](https://developers.deepgram.com)
- **Discord**: [discord.gg/deepgram](https://discord.gg/deepgram)
- **Issues**: [GitHub Issues](https://github.com/deepgram/starter-apps/issues)

## Reporting Issues

Found a bug or have a feature request? Please:

1. Search existing issues to avoid duplicates
2. Create a new issue with a clear title and description
3. Include steps to reproduce the problem
4. Provide your environment details (OS, Go version, etc.)
5. Add any relevant error messages or logs
