// Webpart fork error codes (backend reserves 1090+, see pkg/user/error.go).

// service.enforcetotp is on and the account has not enabled TOTP yet.
export const TOTP_ENROLLMENT_REQUIRED = 1090
