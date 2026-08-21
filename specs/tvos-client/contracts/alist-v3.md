# AList v3 Wire Contract Used by AListTV

Only the fields and routes below are in scope. JSON request keys are snake_case. Requests and responses use UTF-8 JSON.

## Common envelope

```text
{
  "code": Int,
  "message": String,
  "data": Payload | null
}
```

A successful application response has `code == 200`. Relevant error codes are 401 (unauthorized), 402 (OTP required/invalid), 403 (forbidden), and 429 (rate limited). The client examines both HTTP status and envelope code.

Authenticated requests send:

```text
Authorization: <raw token>
Client-Id: <stable UUID>
Accept: application/json
```

`Authorization` is not a Bearer value.

## POST `/api/auth/login`

Headers: `Client-Id`, `Accept: application/json`, `Content-Type: application/json`.

Request:

```text
{
  "username": String,
  "password": String,
  "otp_code": String?   // omitted when absent
}
```

Success data:

```text
{
  "token": String,
  "device_key": String?
}
```

`server/handles/auth.go::LoginReq` defines `otp_code`. `loginHash` returns code 402 for a failed/missing OTP and produces `token` plus `device_key` after ensuring the device session using `Client-Id`.

## GET `/api/me`

No request body. Requires authenticated headers.

The client consumes this subset of success data; additional server fields are ignored:

```text
{
  "id": UInt64,
  "username": String
}
```

## POST `/api/fs/list`

Requires authenticated headers and `Content-Type: application/json`.

Request:

```text
{
  "path": String,
  "page": Int,
  "per_page": Int,
  "refresh": Bool
}
```

Success data consumed by the client:

```text
{
  "content": [AListObject],
  "has_more": Bool?,
  "page": Int?,
  "per_page": Int?
}
```

`has_more`, `page`, and `per_page` are optional for compatibility with older v3 servers. The requested page size is 200; `server/handles/fsread.go` allows at most 500.

## POST `/api/fs/get`

Requires authenticated headers and `Content-Type: application/json`.

Request:

```text
{
  "path": String
}
```

Success data consumed by the client:

```text
{
  "raw_url": String,
  "virtual_path": String?,
  "name": String,
  "size": Int64,
  "is_dir": Bool,
  "modified": String?,
  "created": String?,
  "type": Int?,
  "thumb": String?
}
```

## `AListObject`

The list content representation consumed by the client is:

```text
{
  "virtual_path": String?,
  "name": String,
  "size": Int64,
  "is_dir": Bool,
  "modified": String?,
  "created": String?,
  "type": Int?,
  "thumb": String?
}
```

The app derives a missing `virtual_path` from the requested parent path and `name`; malformed names are rejected. Dates remain raw strings so server precision changes do not invalidate the envelope.

## Repository evidence

- `server/router.go` registers `POST /api/auth/login`, authenticated `GET /api/me`, and authenticated `/api/fs/list` and `/api/fs/get`.
- `server/handles/auth.go::LoginReq` uses `otp_code`; `loginHash` reads `Client-Id` and returns `token` and `device_key`.
- `server/middlewares/auth.go::Auth` reads the raw `Authorization` value; `HandleSession` reads the same `Client-Id` for stable device sessions.
- `server/handles/fsread.go::FsList` normalizes pagination with a 500-item maximum and emits `content`, `page`, `per_page`, and `has_more`.
- `server/handles/fsread.go::FsGet` emits `raw_url`, including local `/p` proxy URLs or storage-provided direct URLs.
