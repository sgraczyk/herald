# Customize Herald's Personality

You can now change how Herald talks, what it focuses on, and what rules it follows -- all from your config file. No rebuilding, no code changes. Just edit `config.json` and restart.

## What changed

Herald has a built-in personality: concise, direct, responds in your language, follows Telegram formatting rules. Previously this was hardcoded. Now you can replace it with your own prompt using the `system_prompt` field in `config.json`.

## How to customize

Add a `system_prompt` field to your `config.json`:

```json
{
  "system_prompt": "You are a friendly cooking assistant. Help the user with recipes, meal planning, and cooking techniques. Be warm and encouraging. Keep responses concise for mobile reading.",
  "telegram": {
    "token_env": "TELEGRAM_TOKEN"
  }
}
```

The value is a plain text string. This becomes the instruction that guides how Herald responds to every message.

## Default behavior

If you leave out `system_prompt` or set it to an empty string, Herald uses its built-in personality:

| Trait | Default behavior |
|-------|-----------------|
| Tone | Concise and direct |
| Language | Responds in whatever language you write in |
| Formatting | Follows Telegram rules (no markdown tables, no headings, uses bold/italic/code) |
| Audience | Optimized for mobile readers |

Existing setups work exactly as before. This feature is entirely opt-in.

## Examples

Here are a few custom prompts to give you ideas.

**A formal assistant:**

```json
{
  "system_prompt": "You are a professional executive assistant. Use formal language, be thorough but organized, and structure responses with bullet points. Always present options when there are multiple approaches. Format for Telegram: use bold for emphasis, no markdown tables, no headings."
}
```

**A domain-specific bot (coding helper):**

```json
{
  "system_prompt": "You are a Go programming assistant. Focus on idiomatic Go, clean code, and practical solutions. When showing code, use code blocks. Explain trade-offs briefly. If a question is not about programming, answer it but keep it short. Format for Telegram: no markdown tables, use bold instead of headings."
}
```

**A different default language:**

```json
{
  "system_prompt": "Jesteś pomocnym asystentem AI o imieniu Herald. Zawsze odpowiadaj po polsku, chyba że użytkownik wyraźnie poprosi o inny język. Bądź zwięzły i bezpośredni. Formatowanie dla Telegrama: nie używaj tabel markdown, używaj pogrubienia zamiast nagłówków."
}
```

## Tips for writing good prompts

**Keep it short.** Your system prompt is sent with every message. A long prompt eats into the context window, leaving less room for conversation history and memories. A few sentences to a short paragraph is ideal.

**Include Telegram formatting rules.** The built-in prompt tells Herald to avoid markdown tables and headings because Telegram does not render them well. If you write your own prompt from scratch, include similar instructions or your messages may look broken. A good baseline to add:

```
Format for Telegram: no markdown tables, no headings (# syntax), use bold and italic for emphasis, use code blocks for code. Keep messages concise for mobile.
```

**Think about mobile users.** Most Telegram users read on their phones. Prompts that encourage brief, scannable responses work better than ones asking for long explanations.

**Be specific.** "Be helpful" is vague. "Answer cooking questions with ingredient lists and step-by-step instructions, keep each step to one sentence" gives Herald clear guidance.

## Prompt length warning

Herald logs a warning at startup if your system prompt exceeds 4000 characters. The prompt still works, but a very long prompt reduces the space available for conversation history and memories in each request. If you see this warning, consider trimming your prompt.

## Interaction with memories

Memories are still injected into every conversation regardless of whether you use a custom prompt. When Herald has stored facts about you (preferences, context from past conversations), those are appended to whatever system prompt is active -- built-in or custom. You do not need to account for memories in your prompt text.

## Do I need to do anything?

No. If you are happy with how Herald behaves today, change nothing. Your setup continues to work exactly as before. The `system_prompt` field is optional and defaults to the built-in personality when omitted.

## Troubleshooting

**My custom prompt is not working.**
Check that your `config.json` is valid JSON. A missing comma or unescaped quote will cause Herald to fail at startup. Use a JSON validator or run `cat config.json | python3 -m json.tool` to verify. After editing, restart Herald for the change to take effect.

**Herald's formatting looks broken after I set a custom prompt.**
Your custom prompt completely replaces the built-in one, including its Telegram formatting rules. Add formatting instructions to your prompt. See the "Tips for writing good prompts" section above for the recommended text to include.

**I see a warning about prompt length.**
Your prompt exceeds 4000 characters. It will still work, but it consumes a significant portion of the context window. This means fewer conversation history messages fit in each request, which can make Herald forget earlier parts of your conversation sooner. Shorten the prompt if possible.

**Herald ignores my prompt and acts normal.**
Make sure the field name is exactly `system_prompt` (with underscore, lowercase). Check that the value is a non-empty string. Restart Herald after saving the file.

## Technical details

For implementation details on how the configurable system prompt works, see [Configurable System Prompt Technical Documentation](../configurable-system-prompt.md).
