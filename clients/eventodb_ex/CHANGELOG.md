# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2024-01-21

### Added

- Initial release
- Stream operations: `stream_write/4`, `stream_get/3`, `stream_last/3`, `stream_version/2`
- Category operations: `category_get/3` with consumer groups and correlation filtering
- Namespace management: `namespace_create/3`, `namespace_delete/2`, `namespace_list/1`, `namespace_info/2`
- System operations: `system_version/1`, `system_health/1`
- Real-time SSE subscriptions: `subscribe_to_stream/3`, `subscribe_to_category/3`
- Optimistic locking with expected version
- Comprehensive error handling with `EventodbEx.Error`
