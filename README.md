# SJA Backend API

A Go-based backend API server for api.sjaplus.top, built with Gin framework. This server handles requests exclusively from sjaplus.top domain with CORS protection.

This project is part of SJA Plus ecosystem. The website is currently under migration and this API will serve as the new backend.

## Installation

Prerequisites:

- Git
- Go

```bash
# 1. clone the repo
git clone https://github.com/remyhuang03/sja-backend.git
cd sja-backend

# 2. Install dependencies:
go mod download
```

## Development

### Running Locally

1. Start the development server:
```bash
go run main.go
```

The server will start on `http://localhost:8080`

2. Test the API:
```bash
curl http://localhost:8080/test
```

Expected response:
```json
{
  "message": "Hello from api.sjaplus.top"
}
```

## Production Deployment

### Building for Production

1. Build the binary:
```bash
go build -o sja-backend main.go
```

2. Set production mode (optional):
```bash
export GIN_MODE=release
```

### Running in Production

1. Run the binary:
```bash
./sja-backend
```

Or run directly:
```bash
go run main.go
```

2. Using PM2 to manage the application
```bash
pm2 start sja-backend --name "sja-backend"
```

