# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.3.0] - 2025-01-24

### Added

- `EventodbKit.SubscriptionHub` - centralized hub for SSE poke distribution
  - Maintains single SSE connection to EventoDB per process
  - Distributes pokes locally to registered consumers by category
  - Reduces N polling connections to 1 SSE connection
- `EventodbKit.SubscriptionHub.Core` - pure functional state machine core
  - Three states: `:connecting`, `:connected`, `:disconnected`
  - Automatic reconnection with exponential backoff (1s to 30s)
  - Fallback polling when SSE is disconnected
  - Health check to detect silent connection failures
- Added `subscribe_to_all/2` to EventodbEx for global SSE subscription

### Changed

- Consumer can now optionally use SubscriptionHub for poke-based updates instead of polling

## [0.2.2] - 2025-01-22

### Added

- `EventodbKit.Config` module for centralized configuration
- `:log_sql` config option to control SQL query logging (default: `false`)

## [0.2.1] - 2025-01-22

### Fixed

- Consumer position tracking: fixed off-by-one bug where consumers would re-fetch and re-check the last processed event on every poll cycle
- `Position.load/4` now returns `nil` for fresh consumers (instead of `0`) to correctly distinguish "no events processed" from "processed event at position 0"

## [0.2.0] - 2025-01-22

### Changed

- **Breaking:** `OutboxSender` no longer accepts `:namespace` option - namespace is now extracted from token automatically
- **Breaking:** `Consumer` no longer requires `:namespace` in init state - namespace is now extracted from token automatically
- Namespace is now decoded from base64url to plaintext (e.g., `"default"` instead of `"ZGVmYXVsdA"`)
- Local database tables now store human-readable namespace values

### Fixed

- `Client.extract_namespace/1` now properly decodes base64url namespace from token

## [0.1.0] - 2024-01-21

### Added

- Initial release
- Outbox pattern with `EventodbKit.Outbox` and `EventodbKit.OutboxSender`
- Consumer pattern with `EventodbKit.Consumer` behaviour
- Consumer position tracking with `EventodbKit.Consumer.Position`
- Idempotency support with `EventodbKit.Consumer.Idempotency`
- Database migrations with `EventodbKit.Migration`
- Event dispatcher macro with `EventodbKit.EventDispatcher`
- Consumer group support for horizontal scaling
