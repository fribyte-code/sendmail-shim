# sendmail-shim

<p><img src="https://raw.githubusercontent.com/fribyte-code/sendmail-shim/main/mascot.png?sanitize=true" width="360"></p>

We had a long streak of troubles in configuring /usr/sbin/sendmail inside of a Docker container to host a legacy website for a customer.
Thus, we created this simple program that mimics some of its behavior.

NOTE: this implementation is explicitly not feature-complete compared to sendmail. It is just enough to send emails, and rewrite the sender and recipient email addresses.

## Technical details

This program is written in Go, only using modules from the standard library, and compiles to a single, statically-linked binary for ease of use.
Provide environment variables at build-time, to make the makefile link these values into the final binary.

To build the program, simply run `make`.
The easiest strategy for environment is probably to prefix them to this command, like `SMTP_SERVER=foo:587 SMTP_USER=me SMTP_PASSWORD=secret LOG_FILE=sendmail-shim.jsonl make`. 

### Supported flags
- -t: get recipients from the message (headers provided on stdin)
- -f: sets the "From" header to postfixed string, f.ex: `-fsender@example.com` makes sender@example.com the email sender.
- -r: same behavior as -f

Email addresses provided independent of the -f/-r flag are interpreted as recipients.

All other flags are completely ignored. Feel free to [submit an issue](https://github.com/fribyte-code/sendmail-shim/issues/new) and a PR to add support for more.

## Development

I recommend installing [smtp4dev](https://github.com/rnwood/smtp4dev) for an easy email test interface.

**Prefix with all required arguments** and run `make --always-make run [appropriate sendmail parameters]`.
This will put you into a reading prompt. Type out headers and email content, then use ^D (ctrl+d) to quit.

Simple example:
```
SMTP_SERVER=localhost:25 SMTP_USER=nobody SMTP_PASSWORD=password LOG_FILE=sendmail-shim.jsonl make --always-make run recipient@example.com
To: recipient@example.com
From: sender@example.com
Content-Type: text/plain

Hi! This is my mail message! Sent from the sendmail shim utility!
Bye!
^D
```

Run `go fmt` to format code properly before committing.

### Possible changes

- [ ] Remove evil hacks in flag parsing
- [ ] Better error handling, though this is not critical as the program is instantiated once per use
- [ ] Switch to a more pass-by-value approach. Multiple functions mutating shared objects scales very poorly

