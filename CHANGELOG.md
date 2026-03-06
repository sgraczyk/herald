# Changelog

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
