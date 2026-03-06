# Fix: Improved Message Handling

**Version:** 0.2.0
**PR:** #88

## Summary

Herald now handles your messages more accurately when sending them to the AI for a response. A bug was causing your message to appear twice in the conversation context sent to the AI, which could lead to lower-quality or repetitive responses. This has been fixed.

## What Was Happening

When you sent a message to Herald, the bot was including your message twice in the conversation it passed to the AI. This meant the AI saw something like:

```
You: What is the weather like today?
You: What is the weather like today?   <-- duplicate
```

This duplication could cause the AI to respond oddly, repeat itself, or place undue emphasis on part of your question. It also used more tokens per conversation turn than necessary.

Additionally, if the AI failed to respond (due to a network issue or provider error), your message was still saved to the conversation history. This left behind orphaned messages with no corresponding response, which could affect future conversations.

## What Changed

- **No more duplicate messages.** The AI now sees each of your messages exactly once, leading to more natural and accurate responses.
- **Lower token usage.** Each conversation turn uses fewer tokens since the duplicate is eliminated. This is particularly relevant when using metered API providers like Chutes.ai.
- **Cleaner history on failures.** If the AI provider fails to generate a response, your message is no longer saved to history. This keeps the conversation record clean and avoids confusing context in future exchanges.

## Do I Need to Do Anything?

No. This fix is entirely transparent. Herald continues to work the same way it always has -- you send a message, you get a response. The improvement happens behind the scenes.

If you previously noticed the AI occasionally repeating itself or giving oddly emphatic responses, this fix should reduce those occurrences.

## Clearing Old History

If you have existing conversations that may contain duplicate messages from before this fix, you can reset your conversation history using the `/clear` command in Telegram. This is entirely optional.
