# rin-proxy TODO

## MVP Complete (2026-02-25)
- [x] Go HTTP proxy (Anthropic <-> Gemini conversion)
- [x] Anthropic passthrough (opus/sonnet/haiku)
- [x] Gemini conversion (request, response, streaming SSE)
- [x] Tool calling + tool_use_id generation/mapping
- [x] JSON Schema cleanup (remove $schema, exclusiveMinimum, etc.)
- [x] Vertex AI OAuth (gcloud token, 50-min cache)
- [x] Environment variable config (GOOGLE_GENAI_USE_VERTEXAI, GOOGLE_CLOUD_PROJECT)
- [x] Makefile targets (proxy, install-proxy, uninstall-proxy)
- [x] launchd plist
- [x] Test script (rin-proxy-test.sh)

## Next Steps
- [ ] Agent Teams field test — verify haiku teammates work via Gemini Flash
- [ ] gcloud token expiry error handling (auto-refresh or clear error message)
- [ ] Gemini API error recovery (429/500 retry, Anthropic fallback)
- [ ] routing_log integration — record Gemini request performance in rin-memory
- [ ] CG mode (ref: moai-adk) — tmux pane isolation for separate lead/teammate model mapping
- [ ] API key auth support (Google AI Studio) — for environments without Vertex AI
- [ ] Image/multimodal conversion
- [ ] Thinking block conversion
