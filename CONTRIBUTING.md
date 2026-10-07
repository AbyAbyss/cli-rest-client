# Contributing to Term REST Client

First off, thank you for considering contributing to Term REST Client! It's people like you that make this project great.

## Code of Conduct

This project adheres to a code of conduct that all contributors are expected to follow. Please be respectful and constructive in all interactions.

## How Can I Contribute?

### Reporting Bugs

Before creating bug reports, please check the issue list as you might find out that you don't need to create one. When you are creating a bug report, please include as many details as possible:

- **Clear title and description**
- **Steps to reproduce** - Be specific!
- **Expected behavior** - What you expected to happen
- **Actual behavior** - What actually happened
- **Screenshots** - If applicable
- **Environment** - OS, Go version, terminal type

### Suggesting Enhancements

Enhancement suggestions are tracked as GitHub issues. When creating an enhancement suggestion, please include:

- **Clear title and description**
- **Use case** - Why is this feature useful?
- **Proposed solution** - How should it work?
- **Alternatives** - Other solutions you've considered

### Pull Requests

1. Fork the repo and create your branch from `main`
2. If you've added code, add tests if applicable
3. Ensure the test suite passes (`make test`)
4. Make sure your code follows the style guidelines
5. Write a clear commit message
6. Submit the pull request

#### Pull Request Process

1. Update the README.md with details of changes if needed
2. Update the CHANGELOG.md with your changes
3. The PR will be reviewed and merged if approved

## Development Setup

### Prerequisites

- Go 1.24 or higher
- Make (optional, for using Makefile)

### Getting Started

```bash
# Clone your fork
git clone https://github.com/AbyAbyss/cli-rest-client.git
cd cli-rest-client

# Add upstream remote
git remote add upstream https://github.com/AbyAbyss/cli-rest-client.git

# Install dependencies
go mod download

# Create a branch for your changes
git checkout -b feature/your-feature-name
```

### Making Changes

1. Make your changes in the appropriate files
2. Test your changes thoroughly
3. Run the linter: `make lint`
4. Format your code: `make fmt`
5. Run tests: `make test`

### Code Style

- Follow standard Go formatting (`gofmt`)
- Use meaningful variable and function names
- Add comments for exported functions and types
- Keep functions small and focused
- Write tests for new functionality

### Commit Messages

- Use the present tense ("Add feature" not "Added feature")
- Use the imperative mood ("Move cursor to..." not "Moves cursor to...")
- Limit the first line to 72 characters
- Reference issues and pull requests liberally

Example:
```
Add support for custom headers

This commit adds the ability to set custom HTTP headers
for requests. Users can now add headers through a new
Headers tab in the UI.

Fixes #123
```

### Testing

- Write unit tests for new functions
- Test edge cases and error conditions
- Ensure all tests pass before submitting PR

### Documentation

- Update README.md if you add new features
- Add code comments for complex logic
- Update CHANGELOG.md with your changes

## Project Structure

- `cmd/` - Main application entry points
- `internal/` - Private application code
  - `app/` - Application logic and orchestration
  - `models/` - Data structures
  - `ui/` - UI components
- `pkg/` - Public libraries that could be used by other projects
- `docs/` - Additional documentation

## Questions?

Feel free to open an issue for any questions about contributing.

Thank you for contributing! 🎉


