# Traffic Inspector

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A powerful HTTP proxy for recording, replaying, and analyzing API traffic.

## Overview

Traffic Inspector is a transparent HTTP proxy tool that sits between clients and servers, allowing you to:

- **Record** HTTP traffic for later analysis
- **Replay** previously recorded responses without hitting backend servers
- **Passthrough** requests to target servers with minimal interference
- **Route** different request paths to different backend servers

## Features

- **Multiple Operation Modes**:
  - Record Mode: Capture all requests and responses
  - Replay Mode: Serve recorded responses without contacting backend servers
  - Passthrough Mode: Forward requests to target servers

- **Path-Based Routing**: Route different paths to different target servers
- **SQLite Storage**: Efficiently store and query captured traffic
- **Graceful Shutdown**: Handle process termination properly
- **Low Memory Overhead**: Efficient buffer management with sync.Pool
- **CLI Interface**: Simple command-line operations for all modes

## Installation

```bash
# Option 1: Install directly
go install github.com/dipjyotimetia/traffic-inspector@latest

# Option 2: Clone and build
git clone https://github.com/dipjyotimetia/traffic-inspector.git
cd traffic-inspector
go build
```

## Quick Start

1. **Configure**: Create a config.json file:

   ```json
   {
     "http_port": 8080,
     "http_target_url": "https://jsonplaceholder.typicode.com",
     "target_routes": [
       {
         "path_prefix": "/api/users",
         "target_url": "https://api.example.com/users"
       }
     ]
   }
   ```

2. **Run**:

   ```bash
   # Start in passthrough mode
   traffic-inspector start
   
   # Start in recording mode
   traffic-inspector start --record
   
   # Start in replay mode
   traffic-inspector start --replay
   
   # Use a custom config file
   traffic-inspector start --config my-config.json
   ```

## Configuration Options

| Option | Description | Default |
|--------|-------------|---------|
| `http_port` | Port for the HTTP proxy server | 8080 |
| `http_target_url` | Default target URL for proxying | (required) |
| `target_routes` | Array of path-based routing rules | [] |
| `sqlite_db_path` | Path to SQLite database file | traffic_inspector.db |
| `recording_mode` | Enable recording mode | false |
| `replay_mode` | Enable replay mode | false |

## Path-Based Routing

Traffic Inspector supports sophisticated routing based on request paths:

```
/todos/*     ─────▶ https://jsonplaceholder.typicode.com
/api/v1/products/  ─────▶ https://api.escuelajs.co
```

## Use Cases

- **API Testing**: Record production traffic and replay for testing
- **Demo Environments**: Create reproducible demos without backend dependencies
- **Development**: Work offline using recorded API responses
- **Debugging**: Analyze HTTP traffic details for troubleshooting
- **Performance Testing**: Capture real traffic patterns for load tests

## Project Structure

- **cmd/**: Command-line interface implementation
  - root.go: Base command and configuration
  - start.go: Start the proxy server
  - version.go: Version information

- **config/**: Configuration handling
  - config.go: Load and validate configuration

- **internal/**: Core implementation
  - **db/**: Database operations for storing traffic
  - **proxy/**: Proxy server implementation

- **main.go**: Application entry point

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License
