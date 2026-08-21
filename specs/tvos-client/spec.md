# AList tvOS Client Specification

This document is the normative behavior source for the tvOS 17 milestone. The terms MUST, MUST NOT, SHOULD, and MAY are interpreted as RFC 2119 requirements. UI wording is informative; states and transitions are normative.

## Connection and authentication

### TVOS-CONN-001 — Server URL boundary

**Given** a user-provided server address, **when** the client validates it, **then** it MUST accept only an absolute URL whose scheme is exactly `https`, whose host is non-empty, and which contains no user, password, query, or fragment. It MUST remove trailing path slashes while preserving a non-default port and a non-root path prefix. It MUST reject HTTP, relative URLs, missing hosts, and userinfo before making a request.

### TVOS-AUTH-001 — Login request

**Given** a validated base URL and credentials, **when** login is submitted, **then** the client MUST `POST <base-path>/api/auth/login` with JSON fields `username`, `password`, and optional `otp_code`, and MUST send the stable `Client-Id` header.

### TVOS-AUTH-002 — Two-factor challenge

**Given** login returns envelope code 402 or HTTP status 402, **when** the response is handled, **then** the client MUST enter the 2FA input state. The next submission MUST reuse the same username, password, and `Client-Id` and MUST add the entered OTP. Password and OTP exist in memory only until success, cancellation, or view-model release.

### TVOS-AUTH-003 — Credential persistence

**Given** login succeeds, **when** the session is committed, **then** the client MUST save the token to Keychain before saving the normalized base URL, username, and stable UUID `Client-Id` to UserDefaults, and only then enter the connected state. It MUST NOT persist password or OTP. Any persistence failure MUST prevent the connected state.

### TVOS-AUTH-004 — Session recovery and authenticated headers

**Given** an authenticated API call, **when** the request is built, **then** it MUST send the token verbatim in `Authorization` and the same stable `Client-Id`. **Given** a token at launch, **when** recovery runs, **then** the client MUST call `GET /api/me`. A 401 MUST delete the token and return to login. Any other error MUST retain the token and expose a retryable recovery error.

## Browsing

### TVOS-BROWSE-001 — Root listing and stable grouping

**Given** a connected session, **when** the root loads, **then** the client MUST request `/api/fs/list` with path `/`, page 1, `per_page` 200, and `refresh` false. Directories MUST precede files while response order remains unchanged within each group.

### TVOS-BROWSE-002 — Navigation, focus, and states

**Given** a directory item, **when** it is opened, **then** its virtual path MUST be formed with POSIX joining rules. Returning to a parent MUST restore focus to the card that opened the child. Empty, loading, retryable failure, and permission-denied (403) states MUST each provide a focusable UI target. Names containing `/` MUST be rejected rather than interpreted as nested paths.

### TVOS-BROWSE-003 — Pagination and deduplication

**Given** a page reports `has_more == true`, **when** focus approaches the final eight items (or the last item when fewer than eight exist), **then** the next page MUST be requested at most once. If `has_more` is absent, a full page MUST imply another page and a short page MUST end pagination; one final empty request is allowed. Items MUST be deduplicated by `virtual_path`, retaining the first occurrence.

## Playback

### TVOS-PLAY-001 — Fresh secure playable URL

**Given** a non-directory item is selected, **when** playback begins, **then** the client MUST call `/api/fs/get` for its `virtual_path`, validate the returned `raw_url` as an absolute HTTPS URL, and pass it to AVKit. Empty, relative, or HTTP URLs MUST produce an unplayable state and MUST NOT downgrade to plaintext. Directory objects MUST be rejected without calling `/api/fs/get`.

### TVOS-PLAY-002 — One-time URL refresh

**Given** the current `AVPlayerItem` first reaches `.failed`, **when** the failure event is observed, **then** the client MUST call `/api/fs/get` exactly once more, validate and replace the item, and seek to the pre-failure position. A second failure in the same play session MUST stop automatic retry and expose the readable underlying error. A new selected object starts a new retry budget.

### TVOS-PLAY-003 — Resume progress

**Given** active playback, **when** ten seconds elapse, playback pauses, or the player exits, **then** position and duration MUST be evaluated for persistence. A finite duration greater than zero and position at least 30 seconds but below 90% MUST be saved and restored on the next play. Progress at or above 90% MUST be deleted. Identity MUST contain the normalized base URL, username, and `virtual_path`; at most the 500 most recently updated records are retained.

## Transport security

### TVOS-SEC-001 — System TLS only

**Given** any client request or media URL, **when** TLS trust is evaluated, **then** the client MUST rely only on the system trust chain. The app MUST NOT add self-signed-certificate bypasses, ATS arbitrary-load/local-network exceptions, or certificate pinning.

## Change rule

When this specification conflicts with code, update this file and the acceptance mapping first, then update tests and implementation. A milestone is incomplete unless every requirement has an implementation symbol and test or explicit manual scenario in `traceability.md`.
