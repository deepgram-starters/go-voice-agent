# Go Voice Agent Starter

Start building interactive voice experiences with Deepgram's Voice Agent API using Python Flask starter application. This project demonstrates how to create a voice agent that can engage in natural conversations using Deepgram's advanced AI capabilities.

## What is Deepgram?

[Deepgram's](https://deepgram.com/) voice AI platform provides APIs for speech-to-text, text-to-speech, and full speech-to-speech voice agents. Over 200,000+ developers use Deepgram to build voice AI products and features.

## Sign-up to Deepgram

Before you start, it's essential to generate a Deepgram API key to use in this project. [Sign-up now for Deepgram and create an API key](https://console.deepgram.com/signup?jump=keys).

## Prerequisites

- Go 1.21 or higher
- A Deepgram API Key
- Modern web browser with microphone support

## Quickstart

Follow these steps to get started with this starter application.

### Clone the repository

1. Go to GitHub and [clone the repository](https://github.com/deepgram-starters/go-voice-agent).

2. Install dependencies:
```bash
go mod tidy
```

3. Set your Deepgram API key:
```bash
export DEEPGRAM_API_KEY=your_api_key_here
```

## Running the Application

Start the Flask server:
```bash
go run main.go
```

Then open your browser and go to:

```
http://localhost:3000
```

- Allow microphone access when prompted.
- Speak into your microphone to interact with the Deepgram Voice Agent.
- You should hear the agent's responses played back in your browser.

## Testing

To test the application run:

```bash
go test -v
```

## Getting Help

- [Open an issue in this repository](https://github.com/deepgram-starters/go-voice-agent/issues/new)
- [Join the Deepgram Github Discussions Community](https://github.com/orgs/deepgram/discussions)
- [Join the Deepgram Discord Community](https://discord.gg/xWRaCDBtW4)

## Contributing

We welcome contributions! Please see our [Contributing Guidelines](./CONTRIBUTING.md) for details.

## Security

For security concerns, please see our [Security Policy](./SECURITY.md).

## Code of Conduct

Please see our [Code of Conduct](./CODE_OF_CONDUCT.md) for community guidelines.

## Author

[Deepgram](https://deepgram.com)

## License

This project is licensed under the MIT license. See the [LICENSE](./LICENSE) file for more info.
