# AListTV Requirement Traceability

| Requirement | Implementation symbol | Test / scenario |
|---|---|---|
| TVOS-CONN-001 | `ServerURLValidator.validate(_:)` | `ServerURLValidatorTests`; HTTPS path-prefix manual login |
| TVOS-AUTH-001 | `AListClient.login(username:password:otpCode:)`, `AListClient.makeRequest` | `AListClientContractTests.testLoginRequestHeadersBodyAndBasePath` |
| TVOS-AUTH-002 | `ConnectionViewModel.submit()`, `ConnectionViewModel.submitOTP(_:)` | `ConnectionViewModelTests.testOTPChallengeRetriesSameCredentials`; `AListTVUITests.testOTPChallengeThenLogin` |
| TVOS-AUTH-003 | `ConnectionViewModel.commitLogin(_:)`, `KeychainCredentialStore`, `ConnectionPreferences` | `ConnectionViewModelTests.testTokenSaveFailureDoesNotConnect`; `KeychainCredentialStoreTests` |
| TVOS-AUTH-004 | `AListClient.currentUser()`, `AppContainer.restoreSession()` | `AListClientContractTests.testAuthenticatedHeaders`; `ConnectionViewModelTests.testRecoveryUnauthorizedDeletesToken`; `testRecoveryTransportFailureRetainsToken` |
| TVOS-BROWSE-001 | `BrowserViewModel.loadInitial()`, `BrowserViewModel.stablePartition(_:)` | `BrowserViewModelTests.testRootRequestAndStableDirectoryPartition`; `AListTVUITests.testLoginThenBrowseRoot` |
| TVOS-BROWSE-002 | `AListPath.join(parent:name:)`, `BrowserViewModel.open(_:)`, `BrowserViewModel.moveToParent()` | `AListPathTests`; `BrowserViewModelTests.testReturnRestoresFocus`; manual focus-state scenario |
| TVOS-BROWSE-003 | `BrowserViewModel.loadNextPageIfNeeded(currentItem:)`, `BrowserViewModel.appendPage(_:)` | `BrowserViewModelTests.testPaginationDeduplicates`; `testLegacyHasMoreFallback`; `testConcurrentThresholdRequestsOnce`; 500+ item scenario |
| TVOS-PLAY-001 | `PlayerCoordinator.play(object:)`, `PlayableURLValidator.validate(_:)` | `PlayerCoordinatorTests.testGetPrecedesItemCreation`; `testRejectsInsecureAndEmptyRawURL`; `/p` manual playback |
| TVOS-PLAY-002 | `PlayerCoordinator.handleFailure(_:)` | `PlayerCoordinatorTests.testFirstFailureRefreshesAndRestores`; `testSecondFailureDoesNotRefresh`; expiring URL scenario |
| TVOS-PLAY-003 | `PlaybackProgressStore.update(identity:position:duration:)`, `PlayerCoordinator.restoreProgress()` | `PlaybackProgressStoreTests`; resume and 90% manual scenarios |
| TVOS-SEC-001 | `ServerURLValidator`, `PlayableURLValidator`, default `URLSession` trust handling | Release Info.plist inspection; self-signed and HTTP negative scenarios |
