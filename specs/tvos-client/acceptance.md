# AListTV Acceptance Map

Automated names are stable contract names. Manual scenarios are required only where AVKit, TLS trust, redirect/range behavior, or physical focus behavior cannot be established by unit tests.

| Requirement | Automated evidence | Simulator / Apple TV acceptance |
|---|---|---|
| TVOS-CONN-001 | `ServerURLValidatorTests` | Enter HTTP, userinfo, query, fragment, and a valid HTTPS path-prefix URL; only the last reaches login. |
| TVOS-AUTH-001 | `AListClientContractTests.testLoginRequestHeadersBodyAndBasePath` | Normal account logs in through a reverse-proxy path prefix. |
| TVOS-AUTH-002 | `ConnectionViewModelTests.testOTPChallengeRetriesSameCredentials` and `AListClientContractTests.testMapsHTTP402ToOTPRequired` | 2FA account exposes OTP once and succeeds with the correct code. |
| TVOS-AUTH-003 | `ConnectionViewModelTests.testTokenSaveFailureDoesNotConnect`, `KeychainCredentialStoreTests` | Relaunch restores server/user without exposing password or OTP. |
| TVOS-AUTH-004 | `AListClientContractTests.testAuthenticatedHeaders`, `ConnectionViewModelTests.testRecoveryUnauthorizedDeletesToken`, `testRecoveryTransportFailureRetainsToken` | Invalidate a server session: app returns to login; offline recovery remains retryable. |
| TVOS-BROWSE-001 | `BrowserViewModelTests.testRootRequestAndStableDirectoryPartition` | Root shows directories before files without reordering either group. |
| TVOS-BROWSE-002 | `AListPathTests`, `BrowserViewModelTests.testReturnRestoresFocus`, `testForbiddenRetryState` | Enter a child and return with Menu; focus returns to the opening card. Empty/loading/403/retry targets accept focus. |
| TVOS-BROWSE-003 | `BrowserViewModelTests.testPaginationDeduplicates`, `testLegacyHasMoreFallback`, `testConcurrentThresholdRequestsOnce`, `testPathChangeIgnoresStaleResponse` | Browse a directory containing more than 500 objects without duplicates. |
| TVOS-PLAY-001 | `PlayerCoordinatorTests.testGetPrecedesItemCreation`, `testRejectsInsecureAndEmptyRawURL` | AList `/p` media starts; an HTTP fixture URL is rejected without a request downgrade. |
| TVOS-PLAY-002 | `PlayerCoordinatorTests.testFirstFailureRefreshesAndRestores`, `testSecondFailureDoesNotRefresh`, `testNewObjectResetsRetryBudget` | Expire the first signed URL; exactly one fresh `/api/fs/get` is observed. |
| TVOS-PLAY-003 | `PlaybackProgressStoreTests` and coordinator resume tests | Play past 30 seconds, exit, reopen and observe resume; play past 90%, reopen and observe no resume. |
| TVOS-SEC-001 | Release Info.plist inspection and URL validator tests | A trusted HTTPS server succeeds; self-signed HTTPS and HTTP raw media fail. Console contains no password, OTP, token, or signed raw URL. |

## UI fixture journeys

The `ui-testing` launch argument runs the real view models and navigation against in-memory network, credential, and player boundaries.

- `AListTVUITests.testLoginThenBrowseRoot`
- `AListTVUITests.testOTPChallengeThenLogin`
- `AListTVUITests.testOpenVideoThenReturnRestoresFocus`

## Real-media matrix

Run on a tvOS 17+ simulator or Apple TV against AList 3.52 or later:

1. `/p` proxy URL: start, pause, seek, exit, and resume.
2. Cloud storage 302 and 307 direct URLs: Range playback, seeking, and return work through AVKit.
3. Media with subtitle and alternate audio tracks: both appear in the system AVKit menus.
4. First temporary URL expired: only one fs/get refresh occurs; a second item failure surfaces an error.
