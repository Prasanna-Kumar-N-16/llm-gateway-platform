# llm-gateway-platform

A production-grade gateway that puts a single, resilient API in front of multiple
LLM providers — Anthropic, OpenAI, and Amazon Bedrock. It handles routing,
automatic fallback, retries, and rate limiting, while attributing cost and
capturing the telemetry needed to evaluate model quality and diagnose failures in
production.

## Why

Teams that ship LLM features quickly outgrow talking to a single provider SDK:

- **Reliability** — providers have outages, rate limits, and latency spikes. A
  gateway lets you fail over automatically instead of failing the request.
- **Cost control** — spend needs to be attributed per team, model, and provider
  before it can be managed.
- **Quality** — model and prompt changes must be measured. The gateway records
  prompt/response pairs so they can be scored for validity and hallucination.
- **Operability** — one place to observe traces, latencies, and failure modes
  across every provider.

## Capabilities

| Area | What it does |
| --- | --- |
| Multi-provider gateway | Unified request/response model across Anthropic, OpenAI, and Bedrock. |
| Resilience | Automatic fallback between providers, bounded retries with backoff, and rate limiting. |
| FinOps | Token counting and cost attribution per provider, model, and caller. |
| Eval harness | Captures prompt/response pairs and scores output validity and hallucination rate across prompt and model versions. |
| Observability | Distributed tracing and a failure-mode view over production traffic. |

## Architecture

```
                 ┌──────────────┐
   client ──────▶│  HTTP / gRPC │
                 │     API      │
                 └──────┬───────┘
                        │
             ┌──────────▼───────────┐
             │   routing engine     │  rate limit · retry · fallback
             └──────────┬───────────┘
                        │
        ┌───────────────┼────────────────┐
        ▼               ▼                ▼
   ┌─────────┐    ┌──────────┐     ┌──────────┐
   │Anthropic│    │  OpenAI  │     │ Bedrock  │
   └─────────┘    └──────────┘     └──────────┘
                        │
        telemetry · cost attribution · eval capture
```

## Repository layout

```
cmd/gateway        Application entrypoint
internal/config    Configuration loading and validation
internal/server    HTTP server, routing, middleware
pkg/provider       Public provider interfaces and adapters
api/openapi        API specifications
deployments        Docker and Kubernetes manifests
scripts            Developer and CI helper scripts
docs               Design and operational documentation
```

## Getting started

Requirements: Go 1.24+.

```bash
# Build
make build

# Run the gateway
make run

# Run tests
make test
```

The server exposes a health endpoint once running:

```bash
curl http://localhost:8080/healthz
```

With at least one provider and a route configured (see `.env.example`), send a
chat request through the gateway:

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "X-Gateway-Caller: team-checkout" \
  -d '{"model":"chat-default","max_tokens":256,"messages":[{"role":"user","content":"hi"}]}'
```

The response includes token usage and its attributed USD cost, computed from
a built-in per-model rate table (override via `GATEWAY_PRICING`, see
`.env.example`). The optional `X-Gateway-Caller` header attributes that cost
to a team, service, or feature in the gateway's logs.

```json
{
  "provider": "anthropic",
  "model": "claude-opus-4-8",
  "content": "...",
  "finish_reason": "stop",
  "usage": {"input_tokens": 12, "output_tokens": 34},
  "attempts": 1,
  "cost": {"input_usd": 0.00018, "output_usd": 0.00255, "total_usd": 0.00273, "known": true}
}
```

## Configuration

Configuration is read from environment variables (see `.env.example`). Provider
credentials are never committed; supply them via the environment or your secret
manager.

## Status

Under active development. See `docs/` for design notes.

## License

MIT — see [LICENSE](LICENSE).
