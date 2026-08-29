//go:build kyse

package mail

import appmail "github.com/arandu-io/examples/app/Mail"

@go
type PasswordResetTextData = appmail.PasswordReset
@endgo

Reset your password

{{-- arandu:begin custom --}}
Your reset code is:

{{ .Code }}

The code is single-use and expires shortly.
{{-- arandu:end custom --}}
