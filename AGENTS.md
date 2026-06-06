# Agent Instructions

Keep the project boring, explicit, and idiomatic Go. Prefer small packages, clear ownership, `context.Context` propagation, wrapped errors, graceful shutdown, and standard-library patterns before adding abstractions or framework-like machinery.

Treat templ as the source of truth for HTML and Datastar as the default interaction layer from the first pass. Use Datastar for form submissions, inline validation states, OTP/reset flows, and precise SSE fragment patches wherever it fits; avoid client-side state managers, SPA routing, or browser-heavy JavaScript unless the user explicitly asks for it.

Use NATS and JetStream deliberately. Name subjects, streams, and KV buckets clearly; keep auth session state, OTP challenges, and password reset tokens in JetStream rather than Postgres tables unless the user changes that decision. Make publishers and consumers idempotent, use stable message IDs where durability matters, and keep event payloads versionable so CQRS projections and event-sourcing experiments can evolve without rewriting history.

Before finishing work, run the narrowest meaningful verification. Regenerate templ/sqlc output when inputs change, keep generated files in sync, and call out any command that could not be run so the next agent conversation starts from honest state.
