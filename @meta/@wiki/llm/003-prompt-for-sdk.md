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
- EACH PHASE has TESTS, that must be passing, before moving on!

SPEC RULES:
- keep the spec compact and clear, avoid fluff and over-engineering!
- clearity and simplicity trumps over-specification!
- assume a capable developer!

analyze the current codebase and create me a document with step how to implement this.
here is the plan for elixir: @meta/@pm/issues/ISSUE004-sdk-elixir.md

store it in "@meta/@pm/issues/ISSUE006-sdk-nodejs.md"


-------------------

i want to build a golang sdk for message db.

Client SDKs process:

- tests: docs/SDK-TEST-SPEC.md
- script: bin/run_sdk_tests.sh
- folder: clients/xxxx

Core principles:
- minimal dependencies
- idiomatic code
- each tests creates its own namespace for isolation
- tests run against live backend server
- EACH PHASE has TESTS, that must be passing, before moving on!

SPEC RULES:
- keep the spec compact and clear, avoid fluff and over-engineering!
- clearity and simplicity trumps over-specification!
- assume a capable developer!

analyze the current codebase and create me a document with step how to implement this.
here is the plan for elixir: @meta/@pm/issues/ISSUE004-sdk-elixir.md

store it in "@meta/@pm/issues/ISSUE007-sdk-golang.md"
