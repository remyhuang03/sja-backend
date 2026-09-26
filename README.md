# SJA Plus Go backend

## Architecture

The backend uses Go's `net/http` package and the pgx PostgreSQL connection pool. Its runtime requires no Node.js, Python, MySQL, or external SVG tools.

| Package | Responsibility |
| --- | --- |
| `internal/analyzer` | Bounded project parsing, block statistics, SVG reports, structural comparison |
| `internal/httpapi` | HTTP routing, upload limits, validation, administrator authentication |
| `internal/store` | PostgreSQL queries, review transactions, embedded versioned migrations |
| `internal/localize` | Chinese, English, and Japanese API/report messages |

Analysis has no mutable request state shared across requests. Iterative traversal handles cycles and shared inputs. ZIP parsing reads only the root `project.json`, without extracting assets. Limits are 48 MiB per source file, 64 MiB for expanded JSON, and 200,000 blocks. Analysis, comparison, and image decoding share two processing slots; excess requests receive HTTP 429. Images are decoded, checked for dimensions, and re-encoded as PNG.

Submissions and published projects share one database. Reviews lock the application row before updating status and creating the showcase entry in one transaction. A unique constraint prevents duplicate publication. Review endpoints require `Authorization: Bearer <ADMIN_TOKEN>`. Internal database errors and disk paths are not exposed to clients.

## Run

Use `compose.yaml` in the adjacent `sja-v3` repository to start PostgreSQL, backend, and frontend together.

Standalone development requires Go 1.27 and PostgreSQL 18:

```sh
export DATABASE_URL='postgresql://sja:password@localhost:5432/sja?sslmode=disable'
export ADMIN_TOKEN="$(openssl rand -hex 32)"
go run .
```

Optional variables are `BACKEND_PORT` (default `8080`) and `DATA_DIR` (default `./var`). A database URL is required. If no administrator key is configured, review endpoints return HTTP 503. Configured keys must contain at least 32 characters. Migrations run automatically on startup. The container runs as a non-root user and stores persistent files under `/data`.

Accepted original projects from analysis and comparison are retained privately for 30 days under `uploads`, then removed by an hourly cleanup task. Multipart temporary files are cleaned when requests finish. SVG reports are retained permanently under `reports`; they have no automatic expiration. Showcase images remain in persistent storage. Sources are saved with random server-generated names and mode `0600`, and have no public download route. Analysis sources share the identifier in their report filename; comparison pairs share an identifier with `_original` and `_compared` suffixes.

## API

| Method | Path | Request and response |
| --- | --- | --- |
| GET | `/api/healthz` | Process liveness |
| GET | `/api/readyz` | PostgreSQL readiness |
| POST | `/api/v2/analyze` | Multipart `file`, `is_sort`, `is_high_rank_cate`; returns a report URL in `token` and statistics in `report` |
| GET | `/api/report-img?stamp=...` | SVG report |
| POST | `/api/v2/compare` | Multipart `original`, `compared`; returns similarity and both reports in `data` |
| POST | `/api/v2/project-display-apply` | Multipart `meta` JSON, `cover`, `avatar`; returns HTTP 201 and an application ID |
| GET | `/api/v2/projects-display?n=5` | Latest approved projects; `n` is 1–100 |
| GET | `/api/v2/project-display-review?limit=20&offset=0` | Administrator-only application list, newest first |
| POST | `/api/v2/project-display-review` | Administrator JSON: `id`, `status`, `notes` |
| GET | `/api/media/{id}/{file}` | Image: `cover.png` or `avatar.png` |

Sort options are `desc`, `asc`, and `none`. Category modes are `top12` and `classic`. Legacy `0` and `1` values remain accepted. Active scripts begin at event hats or custom block definitions recognized by the block catalog. Physical blocks unreachable from active scripts still count toward the total.

The `sja_locale` cookie selects `zh`, `en`, or `ja`, with Chinese as the default. API clients can send `X-SJA-Locale` when no valid cookie is present. Error messages, submission/review confirmations, and generated SVG reports use that locale. Reports retain their original language. User-submitted names, descriptions, and review notes are never automatically translated.

Example `meta`:

```json
{
  "project_name": "Sample project",
  "author_name": "Example author",
  "author_link": "https://scratch.mit.edu/users/example",
  "brief": "A sample project",
  "links": [{"platform": "scratch", "url": "https://scratch.mit.edu/projects/123", "is_default": true}]
}
```

Covers are limited to 5 MiB; avatars to 2 MiB. Accepted formats are JPEG, PNG, and WebP, with each side at most 4096 pixels and at most 12,000,000 pixels total. Provide 1–10 project links and exactly one default. Reviews transition only from `pending` to `approved` or `rejected`; repeated reviews return HTTP 409. Rejections require notes.

Comparison uses multiset Dice coefficients for opcodes and adjacent opcode pairs. It ignores IDs, positions, input constants, assets, and sprite names. The components have equal weight unless both projects have no edges, in which case only opcode similarity is used. This is a structural screening tool, not a semantic comparison or independent proof of plagiarism.

## Tests and releases

```sh
go vet ./...
go test -race ./...
TEST_DATABASE_URL='postgresql://sja:password@localhost:5432/sja_test?sslmode=disable' go test -race ./...
```

Database integration tests skip when `TEST_DATABASE_URL` is unset. They create and clean isolated schemas and verify repeatable migrations, concurrent review, and consistent publication. Use a dedicated test database. GitHub Actions provides PostgreSQL, runs all checks, and deploys verified `main` commits.

Frontend and backend communicate through same-origin `/api`; no CORS allowlist is needed. See the [deployment guide](https://github.com/remyhuang03/sja-v3/blob/main/deploy/README.md). Contact: [me@remya.top](mailto:me@remya.top).
