---
title: "Announcing Opentypist"
date: "2026-05-22"
description: "Opentypist v0.1.0"
---

I am happy to announce my first open source project, [Opentypist](https://github.com/tahashafiei/Opentypist)!

Opentypist is a real-time local AI ghost-text autocomplete for macOS. Suggestions appear inline as you type. It works across any app and is entirely powered on-device.

If you have been tapped into the MacOS software scene (I don't blame you if you are not, I certainly am not), you'll notice that this is not a novel idea. [Cotypist](https://cotypist.app) comes to mind and rightfully so! However as you can imagine there are some differences between Opentypist and Cotypist, namely Opentypist is fully FOSS.

---

## Opentypist v0.1.0

Releasing in a beta state was intentional. Opentypist needs a lot of work but a lot of the core functionality is here. Additionally releasing it in such a state allows others to fork and extend the project to satisfy their own needs.

Right now v0.1.0 offers the following features:

- Ghost-text suggestions rendered as a floating overlay anchored to your cursor
- On-device inference via Apple MLX
- Ollama fallback over localhost:11434 when MLX is unavailable
- Sub-150ms latency through debouncing and in-flight cancellation
- Per-app blacklist — suppresses suggestions in terminals and IDEs
- Menu bar tray — no Dock icon, no clutter

If you want to read more about Opentypist's architecture, you should check out Opentypist's [wiki](https://github.com/tahashafiei/Opentypist/wiki)

---

## End

I will continue to work on Opentypist until the suggestion engine is actually good and usable. Also quick shout out to [Snazzy Labs](https://www.youtube.com/@snazzy) for his [video](https://www.youtube.com/watch?v=npxlJdxSv4A) which drew my attention to Cotypist and inspired me to make my own version.

Okay bye bye.
