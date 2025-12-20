# Race Detection on macOS - Xcode 16 Fix

## The Problem

Xcode 16 SDK has visionOS macros that Go's race detector doesn't understand:

```
error: unknown platform 'visionos' in availability macro
error: pointer is missing a nullability type specifier
```

## The Fix

Add this before running tests:

```bash
export CGO_CFLAGS="-Wno-error=nullability-completeness -Wno-error=availability"
go test -race ./...
```

**Or just use the scripts - they already have the fix:**
- `./bin/qa_check.sh`
- `./bin/race_check.sh`
- `make test-race`
