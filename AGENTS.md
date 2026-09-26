# Repository conventions

- Write documentation, source comments, and commit messages in English.
- Keep API and report translations in `internal/localize`; support Simplified Chinese, Traditional Chinese, English, and Japanese.
- Never translate user-submitted names, descriptions, or review notes.
- Keep database changes versioned and compatible with application rollback.
- Run `gofmt`, `go vet ./...`, and `go test -race ./...`; use a dedicated PostgreSQL database for integration tests.
- Deploy application changes through the existing GitHub Actions workflow on `main`.
- Never commit credentials or generated upload data.
