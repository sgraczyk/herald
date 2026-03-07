# Image Understanding

Herald can look at photos you send and tell you what it sees. You can ask questions about images, get descriptions, or have Herald analyze visual content -- all directly in your Telegram chat.

## Sending Images

Send a photo to Herald in Telegram the same way you would send a photo to anyone else. Herald will automatically detect that you sent an image, analyze it, and reply with a response.

That is all there is to it. No special commands, no setup steps. Just send a photo.

## How It Works

When you send a photo, Herald downloads it, passes it to an AI vision model, and sends the result back to you. The AI model "sees" the image and can describe its contents, read text in it, identify objects, and answer questions about what is shown.

## Using Captions as Prompts

You can add a caption to your photo before sending it. Herald treats the caption as your question or instruction about the image.

**With a caption:** Herald uses your caption as the prompt. For example, if you send a photo of a restaurant menu with the caption "What are the vegetarian options?", Herald will focus on answering that specific question.

**Without a caption:** Herald defaults to describing what is in the image. It will give you a general overview of the contents.

A few examples of useful captions:

- "What does this sign say?"
- "Is this plant healthy?"
- "Translate the text in this image."
- "How many people are in this photo?"

## Requirements

Image understanding requires an OpenAI-compatible vision provider to be configured, such as Chutes.ai. The `claude -p` provider cannot handle images, so Herald automatically routes photo messages to a compatible vision provider.

If no vision-capable provider is configured, Herald will let you know with an error message.

## Limitations

**Images are not saved.** Herald only sees the image for the current message. It will not remember or reference the image in future messages. If you send a follow-up question about the same image, you will need to send the photo again.

**Large images are automatically resized.** To keep things fast and efficient, images larger than 2000 pixels in either dimension are scaled down before processing. This happens automatically and should not affect the quality of the response in most cases.

**Supported formats.** Standard photo formats work best: JPEG and PNG. WEBP images are also supported, but they are sent to the AI model without resizing, so very large WEBP files may be rejected if they exceed the size limit.
