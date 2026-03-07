# Automatic Memory Extraction

Herald now learns about you automatically as you chat. After each conversation exchange, it quietly picks up on facts, preferences, and personal details you mention -- and remembers them for future conversations.

## What changed

Previously, memory extraction happened as part of generating each response, which meant you had to wait a little longer before getting a reply. Now, Herald sends you the response first and then processes the conversation for memorable details in the background. The result: faster replies, same memory capabilities.

## How memories work

Every time you send a message and Herald replies, it reviews the exchange for anything worth remembering. If it finds a durable fact about you -- like a preference, a habit, or a personal detail -- it saves it automatically. The next time you chat, Herald already knows that context and can give you more relevant answers.

These automatic memories are labeled as "auto" to distinguish them from things you explicitly asked Herald to remember.

## What gets remembered

Herald looks for lasting, meaningful facts about you:

- **Preferences** -- your favorite tools, languages, foods, or workflows
- **Personal details** -- your name, job, location, or timezone
- **Habits and routines** -- how you like your code formatted, your work schedule
- **Background info** -- projects you're working on, skills you have

The goal is to remember things that will be useful across many conversations, not just the current one.

## What gets skipped

Not every message triggers memory extraction. Very short messages -- under 10 characters or single words like "ok", "thanks", or "yes" -- are skipped entirely since they rarely contain anything worth remembering.

Herald also avoids saving duplicate facts. If it already knows something about you, it will not store the same thing again.

## Related commands

You can manage your memories manually at any time:

- `/remember <fact>` -- Explicitly tell Herald to remember something (e.g., `/remember I prefer dark mode`)
- `/forget <fact>` -- Remove a stored memory by keyword (e.g., `/forget dark mode`)
- `/memories` -- See everything Herald currently remembers about you

Explicit memories (from `/remember`) are always prioritized in conversations, so use that command for things you definitely want Herald to keep in mind.

## No action needed

This is a behind-the-scenes improvement. There are no new settings to configure and nothing you need to change about how you use Herald. Just chat normally and enjoy the faster responses.
