# British Politics Debate Assistant — Stage 5

A self-hostable debate preparation and analysis workspace focused on British politics. Stage 5 adds browser-based live speech transcription, speaker controls and optional automatic analysis while retaining the preparation, research and evidence-ledger workflows from earlier stages.

## What's included

### Debate workspace
- Create and save debate sessions in SQLite.
- Enter statements for yourself and your opponent.
- Analyse both sides to the same evidential standard.
- Identify potential factual errors, missing context, unsupported claims, contradictions and weak reasoning.
- Generate suggested responses and follow-up questions.
- When web search is configured, retrieve relevant sources for the latest statement and attach source IDs to findings.

### Preparation and research
- Search a topic using complementary queries for official UK policy/statistics/Parliament material and independent evidence/analysis.
- Generate a structured research brief with arguments for and against, key factual claims, expected counterarguments, weaknesses in your position, questions to ask, and caveats.
- Preserve source titles, URLs, domains, publication dates when available, and snippets.
- Save research reports in SQLite and reopen them from Debate history.
- Start a debate session directly from a research brief.

### Stage 3 — Argument and evidence ledger
- Build a persistent register of important claims from the complete debate transcript.
- Attribute each claim to you or your opponent and retain the transcript statements where it appeared.
- Classify claims as factual, predictive, interpretive, value-based or policy proposals.
- Retrieve a limited number of web searches for factual claims and attach source links and relevant extracts.
- Assess evidence as supporting, challenging or providing context for a claim.
- Distinguish supported, contested, contradicted, context-needed, unverified and opinion-based claims.
- Identify potential contradictions within one speaker's statements or between the two speakers.
- Refresh the ledger after adding new transcript entries; the interface flags when the ledger is out of date.
- Persist claims, evidence and contradiction records in SQLite across restarts.

The ledger is updated manually using **Build argument ledger** or **Refresh argument ledger**. It is not automatically run after every transcript entry, which avoids extra model calls and web-search usage. One update analyses up to 100 transcript entries (maximum 120,000 transcript bytes) and searches up to six selected factual/predictive claims.

### Stage 4 — Advanced debate preparation
- Generate a fair devil’s-advocate assessment of the user's position.
- Prepare concise opening and closing statements.
- Build a rebuttal bank with likely challenges, suggested responses, caveats and source references.
- Highlight evidence gaps and practical debate strategy.

### Stage 5 — Live debate intelligence
- Capture speech from the microphone using the browser's Web Speech API (Chrome/Edge are recommended).
- Convert speech into transcript entries and save them to the active debate automatically.
- Choose whether the current speaker is you or your opponent, and correct speaker labels afterwards. Correcting a speaker label clears the derived claim ledger so it can be rebuilt against the corrected transcript.
- Show interim speech while the browser is listening and finalised text in the saved transcript.
- Optionally run AI analysis after a 10-second pause in speech. Automatic checks are limited to one every 30 seconds to reduce unnecessary Gemini and Tavily usage.
- Surface the existing opponent analysis, self-checks, suggested rebuttals, follow-up questions and retrieved source links as the live analysis result.
- Keep manual transcript entry and manual analysis available as fallbacks.

Speaker attribution is user-controlled. The browser speech-recognition interface does not reliably identify which person is speaking, so this release does not claim to provide automatic voice diarisation. Select the speaker before or during capture, then use the transcript's speaker dropdown to correct mistakes.

Microphone access requires browser permission and a secure context (localhost is treated as secure; a deployed site should use HTTPS). Browser speech recognition may process audio through the browser vendor's speech service. The application backend receives and stores recognised text, not raw microphone audio.

## AI and search providers
- **AI:** Google Gemini API, using a key created in [Google AI Studio](https://aistudio.google.com/app/apikey). Default model: `gemini-3.8-flash`.
- **Web search:** Tavily Search API. It is optional for ordinary transcript analysis, but required for the preparation research feature and for live source retrieval in the claim ledger.
- Search results are passed to Gemini as evidence context. The application only accepts source IDs returned by the search provider; model-invented source IDs are discarded.

## Requirements
- Go 1.23+
- Node.js 20+
- A Gemini API key from [Google AI Studio](https://aistudio.google.com/app/apikey)
- A Tavily API key from [Tavily](https://app.tavily.com/home) for web research and claim evidence retrieval

## Gemini reliability settings

The Gemini HTTP client defaults to a 240-second timeout and retries up to two times for transient network errors and HTTP 408, 429, 500, 502, 503 and 504 responses. Configure `GEMINI_TIMEOUT` (Go duration, e.g. `240s`) and `GEMINI_MAX_RETRIES` (0–5). Gemini 3 requests use JSON response mode without a temperature override.

## Configuration

Copy the example environment file:

```bash
cp .env.example .env
```

Set these values in `.env`:

```dotenv
GEMINI_API_KEY=your_google_ai_studio_key
GEMINI_MODEL=gemini-3.8-flash
TAVILY_API_KEY=your_tavily_key
```

The Go API now automatically searches the current working directory and its parent directories for `.env` when it starts. You can run `go run ./cmd/server` from the project root without manually exporting the keys. Existing non-empty environment variables take precedence over values in `.env`. The `.env` file is excluded from Git by `.gitignore`.

`TAVILY_API_KEY` is optional for basic Gemini transcript analysis, but without it:
- the preparation/research feature cannot retrieve sources;
- transcript analysis runs without live source checking and clearly labels that limitation;
- the argument ledger can still extract claims and potential contradictions, but factual claims remain unverified.

## Upgrade from Stage 4

1. Stop the existing API and frontend.
2. Back up the project folder and your SQLite database.
3. Extract this Stage 5 version over the existing project files.
4. Keep your existing `.env` file and database file (for example `data/debates.db`). Do not replace them with the blank `.env.example`.
5. Start the API as described below. The speaker-label update uses the existing messages table; no database migration is required. Existing debates, messages, research briefs and ledger records are retained.

The schema change is additive. No manual SQL migration is required.

## Run locally

### 1. Start the API

From the project root:

```bash
go mod tidy
go run ./cmd/server
```

The API listens on `http://localhost:8080` by default and creates the SQLite database at `./data/debates.db` when `DB_PATH=./data/debates.db` is set, or `./debates.db` otherwise. The `.env.example` sets `DB_PATH=./data/debates.db`.

### 2. Start the frontend

In another terminal:

```bash
cd frontend
npm install
npm run dev
```

Open `http://localhost:5173`.

## Run with Docker

```bash
cp .env.example .env
# Add GEMINI_API_KEY and TAVILY_API_KEY to .env
docker compose up --build
```

Docker Compose reads the project `.env` file for the configured environment variables. The API is exposed on port 8080 and persists its database in the `debate-data` Docker volume. Run the frontend separately during development, or deploy it as a separate static frontend service.

## API endpoints

- `GET /api/health` — provider configuration status (does not reveal secrets).
- `GET /api/debates` — list debate sessions.
- `POST /api/debates` — `{ "title": "...", "topic": "..." }`.
- `GET /api/debates/{id}/messages` — retrieve a transcript.
- `POST /api/debates/{id}/messages` — `{ "speaker": "you|opponent", "content": "..." }`.
- `PATCH /api/debates/{id}/messages/{messageID}` — `{ "speaker": "you|opponent" }`; correct a transcript entry's speaker label.
- `POST /api/analyse` — analyse a transcript; performs a Tavily search when configured.
- `POST /api/research` — `{ "topic": "...", "position": "..." }`; create and save a research brief.
- `GET /api/research` — list saved research briefs.
- `GET /api/research/{id}` — retrieve a saved brief.
- `GET /api/debates/{id}/ledger` — retrieve the saved claim/evidence ledger.
- `POST /api/debates/{id}/ledger/analyse` — analyse the full transcript and refresh the persistent ledger.

## Evidence and accuracy notes

This is an evidence-assisted debate tool, not an automatic fact-checking authority.

- Tavily results include search-result snippets. A snippet is not the same as reading and verifying the complete source document.
- A missing source does not prove a claim is false. The assistant should mark claims as unverified or context-dependent when evidence is insufficient.
- A claim marked supported is supported by the supplied search extract, not independently certified. Open the cited source before relying on a statistic, quotation, legal claim or policy detail.
- Potential contradictions are prompts for review, not definitive findings. Check whether the statements refer to the same policy, date, population, definition and context. A changed position or qualified statement is not necessarily a contradiction.
- For statistics, check dates, definitions, denominators, geographic scope and whether a proposal became law.
- For British politics, distinguish UK-wide matters from England-only policy and areas devolved to Scotland, Wales and Northern Ireland.
- Search rankings are not a guarantee of source quality. Prefer primary sources such as ONS, OBR, Parliament, legislation.gov.uk and the relevant UK or devolved government department.
- The AI can still make mistakes, overlook relevant sources or misinterpret them. Do not present its output as a definitive verdict.

## Cost notes

Gemini API usage limits and model availability depend on the current Google AI Studio tier and can change. Tavily usage depends on the active plan and current credit allowance. The claim ledger intentionally limits web research to up to six selected claims per refresh. Check both provider dashboards and current pricing before deploying or relying on a specific free allowance.

## Current limitations / next stages

- No user authentication, account isolation or role-based access.
- CORS is currently restricted to `http://localhost:5173` for local development.
- No rate limiting, abuse controls, audit trail or production secret-management integration.
- Search snippets are stored with reports and claims; the app does not yet crawl, archive and parse full source documents.
- The ledger is debate-specific and refreshed manually. It does not yet track recurring claims across separate debates or perform continuous background monitoring.
- Speech capture depends on browser support and its speech-recognition service; Chrome or Edge is recommended.
- Speaker labels are selected and corrected manually; automatic speaker diarisation is not included.
- Live analysis is pause-based rather than token-by-token streaming. It uses the existing Gemini/Tavily analysis route and is deliberately rate-limited.

Do not expose this API publicly until authentication, authorization, per-user data isolation, rate limiting, production CORS, logging controls and secret management are implemented. Debate transcripts may contain sensitive personal or political information.

## Suggested next engineering steps

1. Add automatic speaker diarisation using a dedicated speech service or local audio pipeline.
2. Add low-latency streaming analysis and prioritised on-screen alerts.
3. Add authentication and per-user data isolation before public deployment.
4. Add full-page source extraction and source-quality scoring.
5. Add CI, deployment health checks and production observability.
