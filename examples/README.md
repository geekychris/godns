# GoDNS Examples

This directory contains example files and configurations for GoDNS.

## Hosts File Format

The file plugin supports a hosts file format similar to `/etc/hosts`:

```
<ip-address> <hostname1> [<hostname2> ...]
```

### Rules:

- Each line contains an IP address followed by one or more hostnames
- Comments start with `#` and extend to the end of the line
- Empty lines are ignored
- Multiple entries for the same hostname are allowed and will enable round-robin resolution
- Both IPv4 and IPv6 addresses are supported
- Hostnames are case-insensitive

### Example:

```
# This is a comment
192.168.1.10 service1.local
192.168.1.11 service1.local # Multiple entries for service1.local
192.168.1.20 service2.local service2-alias.local # Multiple hostnames for same IP

# IPv6 example
2001:db8::1 ipv6.example.com
```

## Configuration Examples

### Basic Configuration

```yaml
server:
  address: "0.0.0.0"
  port: 53
  cache_ttl: 3600

upstream:
  servers:
    - "8.8.8.8:53"
    - "1.1.1.1:53"
  timeout: 5

plugins:
  file:
    enabled: true
    path: "examples/hosts"
    refresh_interval: 60
  
  memory:
    enabled: true
    entries: []
    
rest:
  enabled: true
  address: "0.0.0.0"
  port: 8080

logging:
  level: "info"
  format: "text"
```

### File Plugin Only Configuration

```yaml
server:
  address: "0.0.0.0"
  port: 53
  cache_ttl: 3600

upstream:
  servers:
    - "8.8.8.8:53"
  timeout: 5

plugins:
  file:
    enabled: true
    path: "examples/hosts"
    refresh_interval: 60
  
  memory:
    enabled: false
    
rest:
  enabled: false

logging:
  level: "info"
  format: "text"
```

### Memory Plugin Only Configuration

```yaml
server:
  address: "0.0.0.0"
  port: 53
  cache_ttl: 3600

upstream:
  servers:
    - "8.8.8.8:53"
  timeout: 5

plugins:
  file:
    enabled: false
  
  memory:
    enabled: true
    entries:
      - hostname: "example.local"
        ips: ["192.168.1.10", "192.168.1.11"]
      - hostname: "api.local"
        ips: ["10.0.0.1"]
    
rest:
  enabled: true
  address: "0.0.0.0"
  port: 8080

logging:
  level: "debug"
  format: "json"
```

