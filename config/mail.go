package config

import framework "github.com/arandu-io/framework/config"

// Mailer is the transport outgoing mail leaves by.
type Mailer string

// The supported mailers.
const (
	// MailerLog writes the message to the log instead of sending it. The
	// development default: a framework that sends real mail from a laptop is a
	// framework that mails a customer during a test.
	MailerLog Mailer = "log"
	// MailerSMTP is any SMTP server.
	MailerSMTP Mailer = "smtp"
	// MailerResend is Resend, and it is what this example is configured for.
	//
	// Resend and SendGrid rather than Amazon SES, and it is a decision rather
	// than an omission: SES needs an AWS account, a sending identity, a region
	// and a sandbox exit request before the first message leaves, and these two
	// need an API key.
	//
	// Both are in the core rather than in a submodule, because neither needs a
	// dependency to reach: one POST with a JSON body is net/http, which every
	// binary already links. See ADR 0031.
	MailerResend Mailer = "resend"
	// MailerSendGrid is SendGrid.
	MailerSendGrid Mailer = "sendgrid"
	// MailerArray keeps messages in memory, for tests to read.
	MailerArray Mailer = "array"
)

// Mail is what config/mail.php holds.
type Mail struct {
	Mailer Mailer

	// Host, Port, Username and Password describe the SMTP server.
	Host     string
	Port     int
	Username string
	Password string

	// Encryption is tls or starttls. There is no "none": an unencrypted SMTP
	// session sends the password in the clear.
	Encryption string

	// Key authenticates with Resend or SendGrid. It is never in the repository:
	// MAIL_KEY comes from the environment, like every other credential.
	Key string

	// FromAddress and FromName are the envelope every message leaves with.
	FromAddress string
	FromName    string
}

func loadMail(base framework.Config) Mail {
	return Mail{
		Mailer:      Mailer(env("MAIL_MAILER", string(MailerLog))),
		Host:        env("MAIL_HOST", "127.0.0.1"),
		Port:        envInt("MAIL_PORT", 587),
		Username:    env("MAIL_USERNAME", ""),
		Password:    env("MAIL_PASSWORD", ""),
		Encryption:  env("MAIL_ENCRYPTION", "starttls"),
		Key:         env("MAIL_KEY", ""),
		FromAddress: env("MAIL_FROM_ADDRESS", "no-reply@localhost"),
		FromName:    env("MAIL_FROM_NAME", base.AppName),
	}
}
