# Skyport Server

Skyport Server is a Go-based tunneling server that enables secure remote access to local services through a web interface.

## Features

- Secure tunnel management
- User authentication and authorization
- Web-based dashboard for tunnel management
- RESTful API for tunnel operations
- Database integration for user and tunnel data

## Prerequisites

- Go 1.19 or later
- PostgreSQL database (for production)
- Environment variables configured

## Installation

1. Clone the repository:
```bash
git clone <repository-url>
cd skyport-server
```

2. Install dependencies:
```bash
go mod download
```

3. Copy environment configuration:
```bash
cp .env.example .env
```

4. Configure your environment variables in `.env`

5. Build the server:
```bash
go build -o skyport-server main.go
```

## Running the Server

```bash
./skyport-server
```

The server will start on the configured port (default: 8080).

## Configuration

Configure the server using environment variables in the `.env` file:

- `PORT`: Server port (default: 8080)
- `DATABASE_URL`: PostgreSQL connection string
- `JWT_SECRET`: Secret key for JWT tokens
- `CORS_ORIGIN`: Allowed CORS origins

## API Endpoints

- `POST /api/auth/login` - User authentication
- `GET /api/tunnels` - List user tunnels
- `POST /api/tunnels` - Create new tunnel
- `DELETE /api/tunnels/:id` - Delete tunnel

## Development

Run in development mode:
```bash
go run main.go
```

## License

[Add your license information here]
