i want to build a nodejs sdk for message db.

Client SDKs process:

- tests: docs/SDK-TEST-SPEC.md
- script: bin/run_sdk_tests.sh
- folder: clients/xxxx

Core principles:
- minimal dependencies
- idiomatic code
- each tests creates its own namespace for isolation
- tests run against live backend server

analyze the current codebase and create me a document with step how to implement this.
here is the plan for elixir: @meta/@pm/issues/ISSUE004-sdk-elixir.md

store it in "@meta/@pm/issues/ISSUE006-sdk-nodejs.md"

