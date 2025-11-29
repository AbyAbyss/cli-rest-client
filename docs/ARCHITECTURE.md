# Architecture

This document describes the architecture of Term REST Client.

## Project Structure

```
term-rest-client/
├── cmd/
│   └── term-rest-client/    # Main application entry point
│       └── main.go
├── internal/
│   ├── app/                 # Application logic (future)
│   ├── models/              # Data models
│   │   └── models.go
│   └── ui/                  # UI components (future)
├── pkg/
│   └── httpclient/         # HTTP client utilities
│       └── client.go
├── docs/                    # Documentation
├── .github/                 # GitHub templates and workflows
└── bin/                     # Build output (gitignored)
```

## Package Overview

### `cmd/term-rest-client`
The main application entry point. Contains the UI setup, event handlers, and application lifecycle.

### `internal/models`
Data structures used throughout the application:
- `RequestTemplate`: Represents a saved HTTP request
- `AppState`: Current application state
- `TreeNodeData`: Data stored in tree nodes

### `pkg/httpclient`
Reusable HTTP client functionality:
- `Client`: Wraps HTTP client with timeout support
- `Response`: Structured HTTP response
- `SendRequest`: Sends HTTP requests
- `FormatResponse`: Formats responses for display

## Design Principles

1. **Separation of Concerns**: UI logic is separated from business logic
2. **Reusability**: HTTP client is in a public package for potential reuse
3. **Maintainability**: Code is organized into logical packages
4. **Extensibility**: Structure allows for easy addition of new features

## Future Improvements

- Move UI components to `internal/ui`
- Extract application logic to `internal/app`
- Add configuration management
- Add persistence layer for collections
- Add plugin system for extensibility

