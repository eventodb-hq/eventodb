# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
