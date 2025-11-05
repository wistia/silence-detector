# silence-detector

A Go library and CLI tool for detecting silence in audio streams.

## Installation

```bash
go get github.com/ssemakov/silence-detector
```

## Building

```bash
go build -o bin/silence-detector ./cmd/silence-detector
```

## Running

```bash
./bin/silence-detector
```

## Testing

```bash
go test ./...
```

## Project Structure

- `cmd/silence-detector/` - Command-line application entry point
- `pkg/detector/` - Public detector library
- `internal/` - Internal packages (not importable by external projects)

## License

MIT