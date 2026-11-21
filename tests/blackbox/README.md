# Mesail blackbox tests

Throwaway acceptance tests written by the architect. They define the
contract from the outside (HTTP and SMTP) and name the SPEC criteria
(`AcCp4` means AC-CP-4). Delete a test only when the slice's own tests cover
the same behavior.

They are behind the `blackbox` build tag, so `go vet ./...` and
`go test ./...` skip them. The gate only compiles them:

    go vet -tags blackbox ./...

To run them against a running Mesail (all-in-one image or `ty preview` of
each app behind the router):

    MESAIL_BASE_URL=http://127.0.0.1:8080 \
    MESAIL_SMTP_ADDR=127.0.0.1:2525 \
    MESAIL_PASSWORD=... MESAIL_PASSWORD_OPERATOR=... \
    MESAIL_SERVICE_CLIENT_SECRET=... \
    MESAIL_TOKEN_ANA=... MESAIL_TOKEN_BO=... MESAIL_TOKEN_EVE=... \
    MESAIL_TOKEN_OPERATOR=... \
    go test -tags blackbox -v ./...

On this Mac, `scripts/native/run-all.sh` starts every app natively and runs
the suite one test at a time. The tokens last 60 seconds, so the fixture
people sign in again before a test whenever their tokens are 30 seconds old
or more (`scripts/floci/test.sh` renews them before each test file).
`MESAIL_TEST_RUN` runs one `-run` filter instead, with one set of tokens. The native runner uses a shared native Keycloak in
`~/Developer/Tools/mesail-keycloak`. Rebuild it after a change in
`deploy/keycloak/dev`, `providers` or `conf` (for example a new provider),
when no other native run uses it:
`MESAIL_KEYCLOAK_REBUILD=1 scripts/native/run-all.sh`
(details: `scripts/native/run-all.sh --help`).

Fixtures they expect (ADR 0005: one login per person, one Keycloak realm
`mesail`, the prefix is the username in every domain). Email crosses
domains; chat never does.

- `ana`: DomainAdmin of `example.com` (her default domain) and a plain
  member of `example.org`. Token `MESAIL_TOKEN_ANA` is her `example.com`
  token.
- `bo`: member of `example.com` (`MESAIL_TOKEN_BO`).
- `eve`: member of `example.org` (`MESAIL_TOKEN_EVE`).
- `carol`: member of `partner.test` only. The seed adds `partner.test` and
  never verifies it, so it stays `pending`: `carol@partner.test` resolves
  `external` (only `active` domains are hosted), and `carol@example.com`
  gets `403 not_a_member` at sign-in.
- `operator`: Keycloak role `mesail-operator`, in no domain
  (`MESAIL_TOKEN_OPERATOR` is an install-wide session, domain `*`).
- `MESAIL_PASSWORD` is the password of the fixture people;
  `MESAIL_PASSWORD_<NAME>` overrides it for one of them (for example
  `MESAIL_PASSWORD_OPERATOR`). The sign-in tests sign in by themselves.
- `MESAIL_SERVICE_CLIENT_SECRET` is the `mesail-service` secret, for the
  service token tests.
- `MESAIL_SIGNIN_LOOP` (a Go duration, default `20s`) is how long
  `TestK2SignInStaysQuickWhileAPersonIsDeleted` signs in as `ana` in a loop
  after deleting a new person; every sign-in must answer 200 within 2
  seconds. Use `240s` or more to cover Keycloak's first session purge
  (about 180 seconds after Keycloak starts; slice K2).
- `MESAIL_K3_MEMBERS` (a number, unset by default) turns on
  `TestK3SignInStaysQuickWhileADomainWithManyMembersIsRemoved`: the
  Operator makes a new domain with that many members (200 for slice K3),
  removes it, and `ana` signs in a loop for `MESAIL_SIGNIN_LOOP` while the
  directory's removal job works; every sign-in must answer 200 within 5
  seconds. Filling 200 members takes a few minutes on Floci, so the test
  skips itself unless the variable is set.
  The same variable turns on
  `TestK6ASmallDomainIsGoneSoonWhileABigOneIsRemovedInMinutes` (slice K6):
  the Operator fills a big domain with that many members and a small one
  with one member, removes the big one, then the small one, and `ana` signs
  in a loop (each sign-in 200 within 5 seconds) until the small domain is
  gone (within 2 minutes) and the big one is gone (within 5 minutes). On
  Floci this needs the scheduler's calls again while `more: true`. Until
  slices K7 and K10 merge, its fill retries a member add once after a
  client timeout (a retry answered `409 duplicate` counts as added); it logs
  each retry, prints the count, and fails when more than 5% of the adds
  needed a retry.

The one-login tests (`SignIn_test.go`, `DomainSwitch_test.go`,
`Tokens_test.go`, `Members_test.go`, `People_test.go`; slice L0) create
new people with prefixes `bb<time>` and new domains `bb<time>.test`.
Prefixes are never reused (SPEC Q5). Since 2026-11-12 every test removes
the domains and the new people it made when it ends (`Cleanup.go`: the
Operator deletes them; the directory finishes in the background), so the
dev stack does not grow with each run. `MESAIL_KEEP_TEST_DATA` (any value)
keeps them, for debugging. A deleted person is never removed from the
directory: `GET /api/v1/directory/people` keeps it with `state: deleted`
(its prefix stays reserved, DOMAIN.md), so a test or tool that counts
people skips that state (on the dev host on 2026-11-21, 150 of 652 people
were deleted). To find people by name, use `?query=`; the route ignores
other parameters such as `text` and then answers the whole list. The K3 load test's 200 members are not deleted
one by one; removing their domain is enough. Wrong-password tests use new people, so the
fixtures are never locked by brute-force detection. The tests that wait
for a membership change allow 70 seconds.

The offline-first tests (`Sync_test.go`, `Queue_test.go`,
`KeptDevice_test.go`; slice O0, ADR 0006) send direct messages from ana to
bo, wait up to 15 seconds for the change log, and create new people for the
session tests, so the fixtures' own sessions are never revoked.

The passkey tests (`Passkeys_test.go`; slice PK0, ADR 0008) use a
software WebAuthn authenticator (`Authenticator.go`: an ES256 key pair, a
made-up credential id, user verification always set, attestation `none`)
that reads `optionsJson` and writes `credentialJson` as WebAuthn Level 3
JSON. The origin in its client data is `https://<rp id>`;
`MESAIL_PASSKEY_ORIGIN` overrides it, and it must be listed in
`identity.passkeyorigins`. Each test makes a new person in `example.com`
(by `ana`) with one passkey labelled `Mac`. `MESAIL_PASSKEY_EXPIRY_TEST`
(any value) turns on the expired-challenge test, which waits 125 seconds.

Fixture tokens live 60 seconds. A test that waits longer than about 45
seconds (a removal, a membership change) takes its token from
`RenewingToken`, which signs in again when the token is 45 seconds old.

The flow tests (`FlowTotp_test.go`, `FlowMailOut_test.go`,
`FlowNotify_test.go`, `FlowDevicebox_test.go`, `FlowRealtime_test.go`,
`FlowJobs_test.go`; slice K33) prove whole flows end to end for the integration run
(MSL-INT). Every name starts with `TestFlow<Area>`, so
`MESAIL_TEST_RUN='^TestFlow'` runs them all and
`MESAIL_TEST_RUN='^TestFlowMailOut'` one flow. They need no new variable.

Mail to an outside address through a relay (`FlowMailOutRelay_test.go`,
slice K34) needs `MESAIL_RELAY_SINK_ADDR`, the `host:port` of the relay
sink's message list (`tests/relaysink`; the Floci run sets it to
`127.0.0.1:12580`), and skips without it.
`TestFlowMailOutRelayToAnOutsideAddressIsDelivered` waits for status
`sent`, then finds the message in the sink with a passing DKIM signature
of `example.com` (selector `mesail`).
`TestFlowMailOutRelayRefusedByTheRelayIsReportedBack` sends to the local
part the sink refuses with `550 5.1.1`
(`MESAIL_RELAY_SINK_REFUSED_LOCAL_PART`, default `refused`) at a fresh
domain, so no suppression from an earlier run applies, and waits for
`bounced`. With a sink, `TestFlowMailOutToAnOutsideAddressIsDeferredWithoutARelay`
skips, because outside mail is then sent.

- TOTP: a new person (made by `ana`) turns on the 2-step code;
  `TotpCode.go` computes RFC 6238 codes from the setup's `secret`. Each
  sign-in waits for a fresh 30-second step (at most 30 seconds), so a
  code is never reused by accident.
- Mail out: an email to `eve@example.org` must reach status `sent` on
  Ana's message and arrive; an unknown mailbox in `example.org` must be
  reported `bounced`; an email to an outside address must become
  `deferred` (Floci's mail-out has no relay; a relay sink and real outside
  delivery come with slice K34). They wait up to 150 seconds (the delivery
  job runs on the scheduler).
- Notify uses `ana` and `bo`; the mute test makes two new people (one
  mutes, one is the control that must be notified), so the fixtures'
  settings never change. Waits are 30 seconds.
- Devicebox makes two new people per test, each with one first device
  (no signature needed), and deletes the devices at the end.
- Realtime opens the socket at `<MESAIL_BASE_URL>/api/v1/realtime/socket`
  with `SocketClient.go`, a small standard-library WebSocket client, and
  answers `reauth`. It waits up to 75 seconds for a pushed frame (the
  Floci relay tick is 60 seconds when drain on write fails).

- Scheduled jobs (`FlowJobs_test.go`, after slice K27: Floci's
  `scheduler` every 60 seconds, `scheduler-fast` every 30 seconds for
  mail-out's delivery and relay and notify's jobs and relay,
  `scheduler-daily` for the changes clean-up; no in-function worker runs
  in the Lambda profile). They never call a job route themselves, except
  the daily clean-up, which they call with an install-wide service token
  because a daily run cannot be waited for. The notify test makes 22 new
  people, because notify writes at most 20 notifications inline and
  `/notify/jobs/run` finishes the rest. The directory's removal job is
  already proven by `TestD2*` in `Removals_test.go`, which wait for the
  scheduler. Needs `MESAIL_SERVICE_CLIENT_SECRET`.

A missing variable skips the test.
