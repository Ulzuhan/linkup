# OIDC permission projection: separate follow-up

The CD preparation preserves an inherited permission-projection limit. It does
not promise immediate rejection after a group or consent change. A real-account
permission experiment is outside this CD task and is not a default prerequisite
for publication, supervised bootstrap or subsequent compatible image updates.
The typed-key correction and SQLite readiness contracts remain required.

## Evidence and conditional impact

The signed historical 0.8.0 source is
`0f9d99a9099e33375e336092e38d7b1740e8b865` (B0). The reviewed corrected source is
`7ed7114eb7e6ccf8e0b4c1052674d27ddb90c775` (B1 preparation).
`authorizeWithProvider` is identical in both snapshots:
[B0 group fallback](https://github.com/Ulzuhan/linkup/blob/0f9d99a9099e33375e336092e38d7b1740e8b865/internal/services/auth_revocation.go#L32-L60)
and [B1 group fallback](https://github.com/Ulzuhan/linkup/blob/7ed7114eb7e6ccf8e0b4c1052674d27ddb90c775/internal/services/auth_revocation.go#L32-L60).
Missing or empty UserInfo groups fall back to the access JWT's groups.

The [synthetic projection tests](../internal/services/auth_permission_projection_test.go)
show that the same valid JWT can retain its old access and cookie-admin groups
when UserInfo returns no groups or an empty list. A fresh non-empty incompatible
group list, provider rejection, expiry or local logout denies access. API keys
do not gain group-based administration. Thus an existing session can retain a
withdrawn permission if the provider still accepts its token and returns no
fresh groups. This is a conditional source/fixture finding, not an observed
production revocation failure or a measured acceptance window.

The local session expires at the earlier of access-token expiry and creation
plus the 12-hour SessionTTL; that code is also unchanged from B0. No refresh
token is stored. The actual remaining token lifetime and the installed
provider's response to permission changes were not measured.

The previously inspected Supabase Auth v2.197.0
[UserInfo route](https://github.com/supabase/auth/blob/v2.197.0/internal/api/oauthserver/handlers.go#L658)
returns standard scoped claims without groups. Its
[authentication middleware](https://github.com/supabase/auth/blob/v2.197.0/internal/api/auth.go#L20)
checks JWT/user/session. Consent revocation is not checked in that UserInfo
path; the [grant revocation route](https://github.com/supabase/auth/blob/v2.197.0/internal/api/oauthserver/handlers.go#L621)
also revokes OAuth sessions. In the historically observed account source
`12431058fe1a6a9c9bccbf1a5f52ba349d992add`,
[adminRevocar](https://github.com/Ulzuhan/kaicorp-account/blob/12431058fe1a6a9c9bccbf1a5f52ba349d992add/internal/httpapi/paneles.go#L628)
calls [RevocarGrants](https://github.com/Ulzuhan/kaicorp-account/blob/12431058fe1a6a9c9bccbf1a5f52ba349d992add/internal/store/store.go#L516),
whose SQL updates consent without deleting provider sessions. These are
different operations. Those historical source snapshots were not revalidated
against live services during this documentation task.

## Follow-up if stronger revocation guarantees are required

Define the required rejection window and the authoritative source of fresh
groups. Review the provider's supported session-revocation or logout delivery
and the app's projection contract before choosing a change. A bounded
real-account experiment may then be authorized as a separate task; it must not
be inferred from normal login, health or synthetic CD checks. No account,
password, membership, consent or session change is part of this preparation.

The OCI label `auth-contract=oidc-subject-v2` means unambiguous API-key subject
ownership and exact key/session revalidation. It does not assert immediate
fresh-group revocation. This follow-up does not permit removing publication,
signature, exact-image, readiness, persistence or recovery gates described in
[the release preparation](README.md).
