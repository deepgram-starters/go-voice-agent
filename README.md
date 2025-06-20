> INSTRUCTIONS

> 1. If your app requires an UI: Copy the entire contents of https://github.com/deepgram-starters/deepgram-starters-ui to the `./static/` folder.

> 2. The configuration of the `deepgram.toml` file, is required so we can include the starter in future onboarding workflows.

> 3. Consistent naming of the project repo is important. Please don't deviate from our standards. Example repo name: [language] [use case] 

> 4. Use the readme template below, don't deviate from it.

> 5. Use the [cursor rules](./.cursor/rules) with [Cursor](https://www.cursor.com/) to help build your starter more quickly!
---

# [Language] [Usecase] Starter

> Write a brief intro for this project.

## What is Deepgram?
> Please leave this section unchanged.

[Deepgram’s](https://deepgram.com/) voice AI platform provides APIs for speech-to-text, text-to-speech, and full speech-to-speech voice agents. Over 200,000+ developers use Deepgram to build voice AI products and features.

## Sign-up to Deepgram

> Please leave this section unchanged, unless providing a UTM on the URL.

Before you start, it's essential to generate a Deepgram API key to use in this project. [Sign-up now for Deepgram and create an API key](https://console.deepgram.com/signup?jump=keys).

## Quickstart

> Detail the manual steps to get started.

e.g.

### Manual

Follow these steps to get started with this starter application.

#### Clone the repository

Go to GitHub and [clone the repository](https://github.com/deepgram-starters/prerecorded-node-starter).

#### Install dependencies

Install the project dependencies.

```bash
npm install
```

#### Edit the config file

> Config file can be any appropriate file for the framework/language. For e.g.
> Node is using a config.json file, while Python is only use .env files

Copy the code from `sample.env` and create a new file called `.env`. Paste in the code and enter your API key you generated in the [Deepgram console](https://console.deepgram.com/).

```json
DEEPGRAM_API_KEY=%api_key%
```

#### Run the application

> If your starter has a UI, it must always run on port 8080

The `dev` script will run a web and API server concurrently. Once running, you can [access the application in your browser](http://localhost:8080/).

```bash
npm start
```

## Issue Reporting

If you have found a bug or if you have a feature request, please report them at this repository issues section. Please do not report security vulnerabilities on the public GitHub issue tracker. The [Security Policy](./SECURITY.md) details the procedure for contacting Deepgram.

## Getting Help

We love to hear from you so if you have questions, comments or find a bug in the project, let us know! You can either:

> be sure to set the repo-name in the issue URL.

- [Open an issue in this repository](https://github.com/deepgram-starters/{repo-name]/issues/new)
- [Join the Deepgram Github Discussions Community](https://github.com/orgs/deepgram/discussions)
- [Join the Deepgram Discord Community](https://discord.gg/xWRaCDBtW4)

## Author

[Deepgram](https://deepgram.com)

## License

This project is licensed under the MIT license. See the [LICENSE](./LICENSE) file for more info.
