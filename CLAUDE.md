# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

PeerDB is a collaborative database and search platform built on PostgreSQL and Elasticsearch.
It provides a document-based knowledge base with a claims-based schema, versioned storage, real-time
collaboration, and an adaptive search interface. The system is designed to support multiple sites
from a single installation with domain-based routing.

Key features:

- **Versioned document store** with full change history
- **Real-time collaboration** with conflict detection
- **Claims-based document schema** supporting 11+ claim types (identifiers, text, relations, amounts, time, files, etc.)
- **Adaptive search UI** that automatically adjusts to data and provides relevant filters
- **Multi-site support** with separate schemas and indices per site

## Development

### Backend (Go)

- `make` - Build the complete application with embedded frontend
- `make build` - Build backend binary with embedded frontend dist files

### Frontend (Node/TypeScript/Vue)

- `npm install` - Install frontend dependencies
- `npm run serve` - Start Vite dev server (runs on port 5173, proxied through backend on 8080)
- `npm run build` - Build frontend for production (output to `dist/`)

### Internationalization (i18n)

- `npm run generate-vue-i18n` - Generate TypeScript definitions for vue-i18n from `src/locales/en.json`

**Important**: The file `src/vue-i18n.d.ts` is automatically generated and should NOT be edited manually.
To update internationalization TypeScript definitions:

1. Modify `src/locales/en.json` with your locale changes
2. Run `npm run generate-vue-i18n` to regenerate the TypeScript definitions
3. The script `generate-vue-i18n.js` uses Vue i18n Global resource schema approach for type safety

### Testing and Quality

- `make test` - Run Go tests with coverage
- `make test-ci` - Run tests with CI output formats
- `npm run test-ci` - Run frontend tests with Vitest (CI mode - exits after completion)
- `npm run test` - Run frontend tests with Vitest (watch mode - never exits, do NOT use in CI)
- `npm run coverage` - Generate frontend test coverage

### Linting and Formatting

- `make lint` - Run Go linter (golangci-lint) with auto-fix
- `make fmt` - Format Go code with gofumpt and goimports
- `npm run lint` - Run ESLint on frontend code
- `npm run lint-style` - Run Stylelint on CSS/Vue files
- `npm run lint-vue` - Run Vue TypeScript compiler check
- `npm run fmt` - Format frontend code with Prettier

### Backend Code Style

- **Comments**: All comments must end with dots for consistency.
- **Error Handling**: When error is `errors.E`, use `errE` as variable name and assertion should be of
  form `require.NoError(t, errE, "% -+#.1v", errE)`.
- **CI Commands**: For backend-only changes, run these commands to match CI validation:
  - `make lint` - Go linter (golangci-lint) with auto-fix
  - `make fmt` - Go code formatting with gofumpt and goimports
  - `make test` - Go tests with coverage
  - `make lint-docs` - Documentation linting (affects whole repo)
  - `make audit` - Go security audit with nancy

### Frontend Code Style

- **Comments**: All comments must end with dots for consistency
- **Import Convention**: Always use `@/` alias for internal imports, never relative paths (`./`, `../`)
- **Import Organization**: Type imports for external packages must be at the top with `import type`, followed by
  empty line, then type imports for local packages, followed by empty line, then regular imports for external packages,
  followed by empty line, then regular imports for local packages; each group sorted in alphabetical order
- **Internationalization**: All user-facing text must use vue-i18n with global scope
  - **useI18n**: Always use `useI18n({ useScope: 'global' })` instead of `useI18n()`
  - **i18n-t components**: Always include `scope="global"` attribute: `<i18n-t keypath="..." scope="global">`
  - Technical terms like "passkey" should be extracted into translatable strings but not translated across languages
  - **Never put HTML in translated strings** - HTML formatting must always be in Vue templates, not translation files
    - ❌ Wrong: `"message": "<strong>Success!</strong> Operation completed"`
    - ✅ Correct: `"message": "{strong} Operation completed"` with `<i18n-t>` template interpolation
- **TypeScript**: Strict typing enabled with vue-i18n message schema validation
- **Formatting**: Always run `npm run fmt` after making changes to maintain consistent code formatting
  - Use double quotes (`"`) for strings, not single quotes (`'`)
  - Multi-line Vue template attributes should break after `>` and before `<` on closing tags
  - Files should end with newlines
  - Consistent spacing and indentation per Prettier configuration
- **CI Commands**: For frontend-only changes, run these commands to match CI validation:
  - `npm run lint` - ESLint with auto-fix
  - `npm run lint-vue` - Vue TypeScript compilation check
  - `npm run lint-style` - Stylelint with auto-fix
  - `npm run fmt` - Prettier formatting
  - `npm run test-ci` - Frontend tests with coverage
  - `make lint-docs` - Documentation linting (affects whole repo)
  - `npm audit` - Security audit

### Development Architecture

- Backend serves as proxy to Vite dev server in development mode (`-D` flag)
- Production builds embed frontend files into Go binary via `embed.FS`
- Hot module replacement works through backend proxy during development
- TypeScript strict mode enabled
- Uses Vue 3 Composition API (Options API disabled via `__VUE_OPTIONS_API__: false`)

### Testing Requirements

- Go tests require `dist/index.html` (dummy file acceptable)
- Both PostgreSQL and Elasticsearch must be running for integration tests
- Use `make test-ci` for coverage reports
- Frontend tests use Vitest with v8 coverage provider

### Development Setup Requirements

- Go 1.25+ required
- Node.js 24+ required
- TLS certificates needed (recommend mkcert for local development)
- CompileDaemon for backend auto-reload during development

## Architecture

### High-Level Structure

```plaintext
Backend (Go HTTP API)
    ├── Store (PostgreSQL) - Versioned key-value store with changesets
    ├── Coordinator - Real-time collaboration session management
    ├── Storage - Chunked file upload management
    └── ES Bridge - Syncs Store changesets to Elasticsearch

Frontend (Vue 3 + TypeScript SPA)
    └── API Client → Backend HTTP endpoints
```

### Core Components

#### 1. **Store** (`store/store.go`)

Generic versioned key-value store with full change history. Uses PostgreSQL as backing storage with JSONB columns.

**Key concepts**:

- **Changesets**: Atomic units of change with metadata
- **Views**: Named branches (main view is primary)
- **Versions**: Identified by changeset ID and structure
- **Patches**: Forward and backward changes (AddClaimChange, SetClaimChange, RemoveClaimChange)

**PeerDB usage**: Stores documents as `json.RawMessage` with `DocumentMetadata`.

#### 2. **Coordinator** (`coordinator/coordinator.go`)

Manages append-only logs for real-time collaboration sessions.

**Lifecycle**: Begin session → Append operations → End/Discard session

**Features**:

- Conflict detection during concurrent edits
- Pagination (max 5000 operations per page)
- PostgreSQL-backed with JSONB columns

**PeerDB usage**: Tracks document editing sessions with conflict detection.

#### 3. **Document Schema** (`document/`)

Claims-based document system supporting 11 claim types:

- **IdentifierClaim**: External IDs (e.g., Wikidata Q-IDs)
- **TextClaim**: Rich text with HTML in multiple languages
- **StringClaim**: Plain text strings
- **RelationClaim**: Relationships to other documents
- **AmountClaim/AmountRangeClaim**: Numeric values with units
- **TimeClaim/TimeRangeClaim**: Timestamps and time ranges
- **FileClaim**: File references
- **ReferenceClaim**: URL references
- **NoValueClaim/UnknownValueClaim**: Missing data markers

**Core structure**:

```go
type D struct {
    CoreDocument  // ID (22-char identifier), Score, Scores
    Mnemonic      // Human-readable identifier
    Claims        // ClaimTypes (collections of claims)
}
```

**Important patterns**:

- Use the Visitor pattern to traverse/manipulate claims
- Claims reference properties (also documents) via `prop.id`
- Built-in properties defined in `document/properties.go` (NAME, TYPE, LABEL, etc.)

#### 4. **Search** (`search/search.go`)

Elasticsearch query builder with session-based filtering.

**Filter types**:

- `RelFilter`: Filter by relation claims
- `AmountFilter`: Filter by numeric ranges
- `TimeFilter`: Filter by time ranges
- `StringFilter`: Text search on string/text claims

**Limits**: Max 1000 search results per query

#### 5. **Storage** (`storage/storage.go`)

Chunked file upload management with begin/append/end lifecycle. Files stored in PostgreSQL with metadata
(size, media type, filename, ETag).

#### 6. **ES Bridge** (`internal/es/bridge.go`)

Listens to Store changesets and synchronizes to Elasticsearch using bulk indexing (2 workers, 1000 bulk actions,
1s flush interval). Non-blocking design continues on individual document failures.

### HTTP API Routes

Defined in `routes.json` using the WAF framework (`gitlab.com/tozd/waf`).

**Document endpoints** (`/d/`):

- `POST /d/create` - Create document
- `POST /d/beginEdit/:id` - Start editing
- `POST /d/saveChange/:session?change=N` - Append change
- `POST /d/endEdit/:session` - Commit session
- `POST /d/discardEdit/:session` - Discard session

**Search endpoints** (`/s/`):

- `POST /s/create` - Create search session
- `GET /s/:id` - Get search UI
- `GET /s/results/:id` - Get search results
- `GET /s/filters/:id` - Get available filters

**File endpoints** (`/f/`):

- `POST /f/beginUpload` - Start upload
- `POST /f/uploadChunk/:session?start=N` - Upload chunk
- `POST /f/endUpload/:session` - Finalize upload

### Configuration

**Config file** (YAML format, see `demos.yml` for example):

```yaml
sites:
  - domain: example.com
    index: example_index
    schema: example_schema
    title: "Example Site"
postgres:
  url: postgres://user:pass@host:5432/db
elastic:
  url: http://localhost:9200
```

**CLI flags** override config file. Run `./peerdb --help` for all options.

### Multi-Site Architecture

Single PeerDB instance can serve multiple sites:

- Each site has separate PostgreSQL schema and Elasticsearch index
- Sites share database pool and ES client
- Routing by domain via WAF framework
- Let's Encrypt automatic TLS certificates per domain

### Frontend (Vue 3 + TypeScript)

- **Framework**: Vue 3 with Composition API and TypeScript
- **Build Tool**: Vite for development and production builds
- **Styling**: Tailwind CSS with custom components
- **Router**: Vue Router for SPA navigation
- **API Layer**: Custom fetch wrappers in `src/api.ts`
- **Internationalization**: Vue-i18n v11 with precompiled messages (English and Slovenian support)

### Frontend Structure

- `src/api.ts` - Backend API client
- `src/document.ts` - Document model (26KB, complex)
- `src/search.ts` - Search logic (32KB)
- `src/time.ts` - Timestamp handling with extended year support
- `src/components/` - Reusable Vue components
- `src/views/` - Page components (Home, DocumentGet, DocumentEdit, SearchGet)

### Database Schema Management

- Schemas auto-created on first run (tables, views, stored procedures)
- Multi-site support via schema prefixes (e.g., "docs" for documents)
- Connection pool uses serializable isolation level
- Custom error handling with request ID and schema tracking

### ElasticSearch Index

- Index configuration embedded in `search/index.json`
- Auto-created on first run if missing
- Run `./peerdb populate` to initialize with core PeerDB properties
