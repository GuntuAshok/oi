# oi: Chat with Ollama models right from your terminal — with automatic model discovery, Markdown and piping support, and saved conversations.

This project is a forked version of the original library [mods](https://github.com/charmbracelet/mods), rebuilt with minimal changes to work exclusively with Ollama. Mods is an excellent library, and I really love the [charm](https://github.com/charmbracelet) stuff — their markdown rendering in the terminal is fantastic! However, the Charm folks are now more into their new [crush](https://github.com/charmbracelet/crush). I still love Mods for its simplicity, but I need features that aren't currently available, which is why this library was built on top of it.

## Installation

Binaries will be made available for Linux, macOS and Windows.

For now, just install with `go`:

```shell
go install github.com/GuntuAshok/oi@latest
```

## What's different?

Here are the changes from the original:

* **Ollama Only:** This project only supports Ollama. All other APIs and their related code have been trimmed.
* **Model Autodiscovery:** Automatically discovers your local Ollama models. You can pick a model immediately, set it as the default without changing any configuration, and the model list updates automatically whenever you pull a new model.
* **Chat Support:** Added a simple, non-TUI chat option so you can continue a conversation directly in REPL style from the terminal using the newly introduced `--chat` option. Just type `oi --chat` and you're good to go.
* **Performance Fix:** Ollama's chat API follows a template with system, user, and assistant roles. Conversations must be sent in that format to properly use the context (KV cache) from earlier messages. If the history isn't formatted this way, Ollama must rebuild the KV cache every time you send a new message — which is probably why `mods` slowed down when continuing a chat. This project ensures the history is formatted correctly, eliminating the performance drop during continued conversations.
* **Dependencies:** No new dependencies were added to the original `mods` library. A few dependencies related to unused APIs have been removed.
* **New Binary Name:** `oi` — The binary is now named `oi`. No particular reason — those two letters are adjacent on the keyboard and easy to type.

## Usage

You can see the original `mods` README for all features and how to use the library. Some of it may not be relevant to this project. I will update this README with everything it supports in due course.

**Note:** Just remember to use `oi` in commands instead of `mods`.

## Disclaimer

This repository provides small, quality-of-life improvements for my personal use of Ollama. I’m not a regular Go developer outside this library, so I’ll keep changes minimal and stable, but it’s not a fully supported production tool. I still need to test a few things.

## Contributions

Bug fixes and small, well-documented improvements are always welcome!
