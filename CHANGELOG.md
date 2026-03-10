# Changelog

## [0.4.0](https://github.com/sgraczyk/herald/compare/v0.3.1...v0.4.0) (2026-03-10)


### Features

* **config:** embed config.json.example as built-in defaults ([#109](https://github.com/sgraczyk/herald/issues/109)) ([ac2a5e4](https://github.com/sgraczyk/herald/commit/ac2a5e4667ee44d406f2dc2a90f59e05e84c0095))


### Bug Fixes

* **provider:** use per-chute subdomain URL for Chutes.ai ([#107](https://github.com/sgraczyk/herald/issues/107)) ([3a41d9a](https://github.com/sgraczyk/herald/commit/3a41d9afc68a66e6aa98ccf0b109c226ec796602))
* **telegram:** add nil check for Message.From in handleUpdate ([#125](https://github.com/sgraczyk/herald/issues/125)) ([88eea0f](https://github.com/sgraczyk/herald/commit/88eea0fb9c4a73ec75ca17f17108dbe2488ec206))


### Performance Improvements

* **format:** reuse singleton goldmark instance ([#129](https://github.com/sgraczyk/herald/issues/129)) ([c191015](https://github.com/sgraczyk/herald/commit/c19101596d2580b635063c74957838775dce2fb6))

## [0.3.1](https://github.com/sgraczyk/herald/compare/v0.3.0...v0.3.1) (2026-03-08)


### Bug Fixes

* **provider:** replace dead Chutes.ai model and add startup validation ([#103](https://github.com/sgraczyk/herald/issues/103)) ([2123f59](https://github.com/sgraczyk/herald/commit/2123f59f50d950911cb00bc05b95a76a85566ad2))

## [0.3.0](https://github.com/sgraczyk/herald/compare/v0.2.1...v0.3.0) (2026-03-07)


### Features

* **agent:** make memory extraction async ([#97](https://github.com/sgraczyk/herald/issues/97)) ([270de3a](https://github.com/sgraczyk/herald/commit/270de3a43c42cb6e5bb63e8c9d3fc93d70a324d1))
* **provider:** add image understanding via OpenAI vision API ([#98](https://github.com/sgraczyk/herald/issues/98)) ([d175d07](https://github.com/sgraczyk/herald/commit/d175d07b3d1e87e75db2ab1f9d6530ae2bcc0f9d))


### Bug Fixes

* **health:** return dynamic provider name from /health endpoint ([#94](https://github.com/sgraczyk/herald/issues/94)) ([8569663](https://github.com/sgraczyk/herald/commit/856966387652582eb414a90e2d55af5e2a368112))
* **provider:** detect auth errors and add timeout to OpenAI provider ([#101](https://github.com/sgraczyk/herald/issues/101)) ([debb2bd](https://github.com/sgraczyk/herald/commit/debb2bd32912ec39314a80b7e833e063a405a296))
* **provider:** limit OpenAI response body size to 10 MB ([#93](https://github.com/sgraczyk/herald/issues/93)) ([f44702e](https://github.com/sgraczyk/herald/commit/f44702e220db184a618edfbebe17d45769aeb492))
* **telegram:** preserve full text for unknown commands ([#91](https://github.com/sgraczyk/herald/issues/91)) ([ab3e9a5](https://github.com/sgraczyk/herald/commit/ab3e9a56c8e50f0e2910a2bd33ed4e780d3a31b0))

## [0.2.1](https://github.com/sgraczyk/herald/compare/v0.2.0...v0.2.1) (2026-03-06)


### Bug Fixes

* **agent:** prevent duplicate user message in LLM context ([#88](https://github.com/sgraczyk/herald/issues/88)) ([df1c62c](https://github.com/sgraczyk/herald/commit/df1c62c5e3161090281b1a88b146db3dea452b81))
* **telegram:** reject empty whitelist at adapter construction ([#90](https://github.com/sgraczyk/herald/issues/90)) ([7756543](https://github.com/sgraczyk/herald/commit/7756543fd024e9f15a5a231939d05a709f1d9fb2))

## [0.2.0](https://github.com/sgraczyk/herald/compare/v0.1.2...v0.2.0) (2026-03-06)


### Features

* **agent:** add long-term memory with commands and auto-extraction ([#58](https://github.com/sgraczyk/herald/issues/58)) ([b755853](https://github.com/sgraczyk/herald/commit/b755853141c9d222ac981d12a7bbb8fd3fcbe274))
* **agent:** make system prompt configurable via config.json ([#66](https://github.com/sgraczyk/herald/issues/66)) ([56c6617](https://github.com/sgraczyk/herald/commit/56c6617c0521e023f33a093bc043f6221c89e601))
* **format:** split long responses to respect Telegram message limit ([#64](https://github.com/sgraczyk/herald/issues/64)) ([0390111](https://github.com/sgraczyk/herald/commit/0390111bdef27694dd60c383b92719445f98006f))
* **logging:** replace log.Printf with structured slog ([#65](https://github.com/sgraczyk/herald/issues/65)) ([7fab940](https://github.com/sgraczyk/herald/commit/7fab940764413a74aeaa8c1c87047c4d48bed5fe))
* **telegram:** send typing indicator while preparing response ([#63](https://github.com/sgraczyk/herald/issues/63)) ([53b2e69](https://github.com/sgraczyk/herald/commit/53b2e69f1169020c61adfac82fa2a9267aa8e31f))


### Bug Fixes

* **ci:** combine release build into release-please workflow ([#60](https://github.com/sgraczyk/herald/issues/60)) ([7681275](https://github.com/sgraczyk/herald/commit/76812755a9c926c359125ce8c066e24dbf3dd651))
* **release:** restore manifest to 0.1.2 so next release is 0.2.0 ([f4252ea](https://github.com/sgraczyk/herald/commit/f4252eab9ae4a28142c92e515bbbf3a28e656148))

## [0.1.2](https://github.com/sgraczyk/herald/compare/v0.1.1...v0.1.2) (2026-03-05)


### Features

* **agent:** improve error messages shown to Telegram users ([#55](https://github.com/sgraczyk/herald/issues/55)) ([d5427a9](https://github.com/sgraczyk/herald/commit/d5427a96bd77382253a6ff6c83f5ec8682a818cd))
* **format:** convert markdown to Telegram HTML before sending ([#54](https://github.com/sgraczyk/herald/issues/54)) ([b664996](https://github.com/sgraczyk/herald/commit/b6649962f3b183cf948893d55395f85aa6bf6f87))
* **provider:** detect and report Claude CLI auth failures ([#52](https://github.com/sgraczyk/herald/issues/52)) ([279e74f](https://github.com/sgraczyk/herald/commit/279e74f204382056288a7bc0cc3ee6e3e8c2d49a))


### Bug Fixes

* **provider:** replace removed Chutes.ai fallback model ([#51](https://github.com/sgraczyk/herald/issues/51)) ([7325327](https://github.com/sgraczyk/herald/commit/73253274b87979ccc7ea5fd37cf9df641069833d))
