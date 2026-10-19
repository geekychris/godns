<p align="center">
  <img src="assets/logo.png" alt="Godns logo" width="180">
</p>

# GoDNS

## Project Overview

GoDNS is a flexible, plugin-based DNS server written in Go. It provides DNS resolution services with the ability to:

1. Forward and cache DNS queries to upstream DNS servers
2. Resolve hostnames through customizable plugins
3. Support multiple resolution strategies including round-robin load balancing
4. Provide a REST API for dynamic DNS management

The server is designed to be highly extensible, allowing developers to implement custom resolution strategies for specific hostname patterns, while falling back to traditional DNS resolution for others.

## Features

- **DNS Forwarding**: Forward DNS queries to configurable upstream DNS servers
- **Caching**: Cache DNS responses to reduce latency and external queries
- **Plugin System**: Extensible plugin architecture for custom hostname resolution
- **Pattern Matching**: Route DNS queries to specific plugins based on hostname patterns
- **Multiple Resolution Strategies**: Support for round-robin and other load balancing techniques
- **REST API**: HTTP interface for runtime configuration and DNS entry management
- **Metrics and Monitoring**: Performance and health metrics for observability
- **Configurable Logging**: Detailed logging with adjustable verbosity

## Architecture

GoDNS is structured as a modular application with the following components:

```
godns/
├── cmd/
│   ├── godns/          # Server executable entry point
│   └── godns-client/   # Test client
├── internal/
│   ├── cache/          # DNS response caching
│   ├── config/         # Configuration handling
│   ├── forward/        # DNS forwarding logic
│   ├── plugin/         # Plugin interface and management
│   ├── resolver/       # Core DNS resolution logic
│   ├── rest/           # REST API implementation
│   └── server/         # DNS server implementation
├── pkg/
│   └── api/            # Public API for plugin development
└── plugins/
    ├── file/           # File-based DNS resolution
    └── memory/         # In-memory DNS resolution with REST management
```

The DNS resolution process follows these steps:

1. The server receives a DNS query
2. It checks if the requested hostname matches any plugin patterns
3. If a match is found, the query is forwarded to the appropriate plugin
4. If no match is found or if the plugin cannot resolve, the query is forwarded to the upstream DNS server
5. The response is cached (if caching is enabled) and returned to the client

## Plugin System

GoDNS supports a flexible plugin system that allows for custom hostname resolution. Plugins implement the following interface:

```go
type Resolver interface {
    // CanResolve determines if this plugin can resolve the given hostname
    CanResolve(hostname string) bool
    
    // Resolve returns IP addresses for the given hostname
    Resolve(hostname string) ([]net.IP, error)
    
    // Name returns the plugin's name
    Name() string
}
```

Plugins are registered with the server at startup and are consulted in order of registration when resolving hostnames.

### Built-in Plugins

GoDNS comes with two example plugins:

1. **File Plugin**: Resolves hostnames based on entries in a hosts-style file
2. **Memory Plugin**: Maintains an in-memory registry of hostnames and IP addresses, configurable via a REST API
## Building and Running

### Prerequisites

- Go 1.18 or later
- Make (optional, for using the Makefile)

### Building from Source

```bash
# Clone the repository
git clone https://github.com/yourusername/godns.git
cd godns

# Install dependencies
make install

# Build the server and client
make build

# Alternatively, use Go commands directly
go build -o bin/godns cmd/godns/main.go
go build -o bin/godns-client cmd/godns-client/main.go
```

### Running the Server

```bash
# Generate a default configuration file
make config

# Run with the generated configuration
make run

# Or run manually
./bin/godns -config config.yaml
```

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage report
make coverage

# Run linter
make lint

# Format code
make fmt
```

### Development Setup

1. Install dependencies:
   ```bash
   make install
   ```

2. Create a configuration file:
   ```bash
   make config
   ```

3. Modify the configuration file as needed:
   ```bash
   # Edit the configuration
   vim config.yaml
   ```

4. Build and run the server:
   ```bash
   make build
   ./bin/godns -config config.yaml
   ```

5. Run the client to test:
   ```bash
   # Query a hostname
   ./bin/godns-client query example.com
   
   # Add a DNS entry
   ./bin/godns-client add test.local 192.168.1.10 192.168.1.11
   ```

### Docker Support

```bash
# Build the Docker image
docker build -t godns .

# Run the container
docker run -p 53:53/udp -p 8080:8080 -v $(pwd)/config.yaml:/app/config.yaml godns
```
```

## Configuration

GoDNS is configured using a YAML file. By default, it looks for `config.yaml` in the current directory, but you can specify a different file using the `-config` flag.

Example configuration:

```yaml
server:
  # DNS server settings
  address: "0.0.0.0"
  port: 53
  # Time-to-live for cached entries in seconds
  cache_ttl: 3600

upstream:
  # Upstream DNS servers
  servers:
    - "8.8.8.8:53"
    - "1.1.1.1:53"
  # Timeout for upstream queries in seconds
  timeout: 5

plugins:
  # Plugin configurations
  file:
    enabled: true
    path: "/etc/godns/hosts"
    refresh_interval: 60  # Seconds
  
  memory:
    enabled: true
    # Default entries to load
    entries:
      - hostname: "example.local"
        ips: ["192.168.1.10", "192.168.1.11"]
    
rest:
  # REST API settings
  enabled: true
  address: "0.0.0.0"
  port: 8080

logging:
  level: "info"  # debug, info, warn, error
  format: "text" # text or json
```

## Extending the System

### Creating a New Plugin

To create a new plugin, implement the `Resolver` interface from the `pkg/api` package:

1. Create a new directory under `plugins/` for your plugin
2. Implement the `Resolver` interface
3. Add a registration function in your plugin
4. Import and register your plugin in `cmd/godns/main.go`

Here's a simple example:

```go
package myplugin

import (
	"net"
	
	"github.com/yourusername/godns/pkg/api"
)

type MyPlugin struct {
	// Your plugin configuration and state
}

func (p *MyPlugin) CanResolve(hostname string) bool {
	// Implement logic to determine if your plugin can resolve this hostname
	return strings.HasSuffix(hostname, ".example")
}

func (p *MyPlugin) Resolve(hostname string) ([]net.IP, error) {
	// Implement resolution logic
	return []net.IP{net.ParseIP("192.168.1.1")}, nil
}

func (p *MyPlugin) Name() string {
	return "myplugin"
}

// NewMyPlugin creates a new instance of MyPlugin
func NewMyPlugin(config map[string]interface{}) (api.Resolver, error) {
	// Parse configuration and initialize your plugin
	return &MyPlugin{}, nil
}
```

### Using the REST API for Runtime Management

The REST API allows for runtime configuration and management:

```bash
# Add or update a DNS entry
curl -X PUT http://localhost:8080/api/entries \
  -H "Content-Type: application/json" \
  -d '{
    "hostname": "service.local",
    "ips": ["10.0.0.1", "10.0.0.2"]
  }'

# Query an entry
curl http://localhost:8080/api/entries/service.local

# Delete an entry
curl -X DELETE http://localhost:8080/api/entries/service.local
```
## Examples

### Basic Usage

```bash
# Start the server with default configuration
./bin/godns

# Query a hostname using the test client
./bin/godns-client query example.com

# Query a hostname using dig
dig @localhost example.com
```

### Using the File Plugin

Create a hosts file at `/etc/godns/hosts`:
```
192.168.1.10 service1.local
192.168.1.11 service1.local
192.168.1.20 service2.local
```

Update your configuration to enable the file plugin:
```yaml
plugins:
  file:
    enabled: true
    path: "/etc/godns/hosts"
    refresh_interval: 60
```

### Using the Memory Plugin with REST API

Start the server with the memory plugin enabled:
```yaml
plugins:
  memory:
    enabled: true
```

#### Using the Client Tool with the Memory Plugin

```bash
# Add a new DNS entry
./bin/godns-client add api.local 10.0.0.1 10.0.0.2 10.0.0.3

# List all DNS entries
./bin/godns-client list

# Get a specific DNS entry
./bin/godns-client get api.local

# Delete a DNS entry
./bin/godns-client delete api.local

# Query a hostname
./bin/godns-client query api.local
```

#### Using curl with the REST API

Add entries using the REST API:
```bash
curl -X PUT http://localhost:8080/api/entries \
  -H "Content-Type: application/json" \
  -d '{
    "hostname": "api.local",
    "ips": ["10.0.0.1", "10.0.0.2", "10.0.0.3"]
  }'

# List all entries
curl http://localhost:8080/api/entries

# Get a specific entry
curl http://localhost:8080/api/entries/api.local

# Delete an entry
curl -X DELETE http://localhost:8080/api/entries/api.local
```

Query the hostname:
```bash
dig @localhost api.local
```

The server will rotate through the IP addresses in a round-robin fashion.

### Client Tool Options

The client tool supports various options:

```bash
./bin/godns-client --help

# Options:
#   -dns-server string
#        DNS server address (default "localhost")
#   -dns-port int
#        DNS server port (default 53)
#   -rest-server string
#        REST API server address (default "localhost")
#   -rest-port int
#        REST API server port (default 8080)
#   -timeout duration
#        Timeout for operations (default 5s)
```

Example with custom options:
```bash
./bin/godns-client -dns-server 192.168.1.10 -dns-port 5353 query example.com
```
The server will rotate through the IP addresses in a round-robin fashion.

---

## License

This project is licensed under the MIT License - see the LICENSE file for details.

