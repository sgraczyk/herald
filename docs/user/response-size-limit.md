# Response Size Limit

*Applies to Herald v0.2.2+ (PR #93)*

## What Changed

Herald now limits the size of responses it accepts from external AI providers
(such as Chutes.ai, Groq, or any other OpenAI-compatible API). If a provider
sends back a response larger than 10 MB, Herald will reject it and try the
next provider in the fallback chain.

## Why It Matters

Herald is designed to run on minimal hardware -- a small container with 512 MB
of RAM. Without a size limit, a misbehaving or compromised upstream API could
send an unexpectedly large response and exhaust all available memory, crashing
the bot. This safeguard prevents that scenario and keeps Herald running
reliably.

## What You Will See if the Limit Is Hit

If a response exceeds the limit, Herald treats it the same as any other
provider failure: it moves on to the next provider in the fallback chain. If
all providers fail, you will receive a short error message in Telegram
indicating that the request could not be completed. No partial or corrupted
response will be delivered.

## The 10 MB Threshold

The limit is set at 10 MB per response. For context, a typical AI chat
response is a few kilobytes at most -- well under one percent of this limit.
Even an unusually long, detailed answer would not come close to 10 MB. This
threshold exists solely as a safety net against abnormal upstream behavior, not
as a constraint on everyday use.

## No Action Required

This change is fully transparent under normal conditions. There are no new
settings to configure and no changes to how you interact with Herald. The
protection is automatic and built in.
