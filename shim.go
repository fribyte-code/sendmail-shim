package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/smtp"
	"os"
	"regexp"
	"strings"
	"time"
)

// global variables, values linked in at build-time
var SMTP_SERVER string
var SMTP_USER string
var SMTP_PASSWORD string
var LOG_FILE string

type EmailAddress struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

func (a *EmailAddress) StringFormat() string {
	if a.Name != "" {
		return fmt.Sprintf("%s <%s>", a.Name, a.Address)
	}

	return a.Address
}

func FormatAddresses(as *[]*EmailAddress) string {
	x := ""
	for i, v := range *as {
		if i != 0 {
			x += ","
		}
		x += v.StringFormat()
	}

	return x
}

func ParseAddresses(addrs string) []*EmailAddress {
	// this regex looks for a name (string, may include spaces), then an email inside of angle brackets
	// it finds all instances of this structure in a comma-separated string.
	// for pure emails without a name first, these are identified as names.
	// This not optimal as it accepts addresses with spaces in them
	re := regexp.MustCompile(`(?P<name>[^<>,]+)?(?:\s*<\s*(?P<address>[^<>,]+)>)?(?:,\s*)?`)
	matches := re.FindAllStringSubmatch(addrs, -1)

	// might want to deduplicate in the future
	x := []*EmailAddress{}
	for _, match := range matches {
		a := EmailAddress{
			Name:    strings.Trim(match[1], " "),
			Address: strings.Trim(match[2], " "),
		}

		// The regex parses unnamed addresses as names. If no address (second field), then swap them
		if a.Address == "" {
			a.Address = a.Name
			a.Name = ""
		}

		x = append(x, &a)
	}

	return x
}

type Email struct {
	Timestamp time.Time `json:"timestamp"`

	From    *EmailAddress   `json:"from"`
	ReplyTo []*EmailAddress `json:"replyTo"`
	To      []*EmailAddress `json:"to"`
	CC      []*EmailAddress `json:"cc"`
	BCC     []*EmailAddress `json:"bcc"`
	Subject string          `json:"subject"`

	ExtraHeaders string `json:"extraHeaders"`
	Body         string `json:"body"`
}

func (e *Email) PopulateFromArgs(args []string) {
	// matches -f or -r arguments that are postfixed with an email address
	// note that there's no space between the flag and the address
	// captures only the email itself
	captureSender, err := regexp.Compile(`(?:-[fr])([^@\s]+@[^@\s]+)`)
	if err != nil {
		panic(err)
	}

	argString := []byte(strings.Join(args, " "))

	maybeSender := captureSender.FindAll(argString, -1)
	if maybeSender != nil {
		// if specified multiple times, last takes precedence
		addr := string(maybeSender[len(maybeSender)-1])[2:] // ugly slice hack
		e.From = &EmailAddress{Address: addr}
	}

	// -t flag: get recipients from the message body
	for _, v := range args {
		if v == "-t" {
			return
		}
	}

	// find recipients (regex matches email addresses loosely)
	emailRegex, err := regexp.Compile(`([^@\s]+@[^@\s]+)`)
	for _, v := range args {
		// ugly hack
		if len(v) != 0 && v[0] != '-' && emailRegex.Match([]byte(v)) {
			e.To = append(e.To, &EmailAddress{Address: v})
		}
	}
}

func (e *Email) PopulateFromStdin(stdin *os.File) {
	b, err := io.ReadAll(stdin)
	if err != nil {
		panic(err)
	}

	content := string(b)
	if !strings.Contains(content, "\r\n") {
		// more stable cli
		content = strings.ReplaceAll(content, "\n", "\r\n")
	}

	s := strings.SplitN(content, "\r\n\r\n", 2)
	if len(s) == 0 {
		panic("no well-formatted body provided")
	}
	if len(s) == 1 {
		e.Body = s[0]
		return
	}

	headers := s[0]
	e.Body = s[1]

	rows := strings.Split(headers, "\r\n")
	// upon encountering From, Reply-To, To, CC, BCC, Subject: header and put in struct.
	// remove sender header from ExtraHeaders
	for i, v := range rows {
		comp := strings.SplitN(v, ":", 2)
		if len(comp) != 2 {
			if i != 0 {
				panic("Malformed header section")
			}

			// recovery: no headers provided
			e.Body = content
			break
		}

		name := comp[0]
		val := strings.Trim(comp[1], " ")

		switch strings.ToLower(name) {
		case "from":
			if e.From == nil {
				a := ParseAddresses(val)
				if len(a) != 1 {
					panic("No or multiple From-addresses given")
				}

				e.From = a[0]
			}
		case "sender": // ignored
		case "reply-to":
			if len(e.ReplyTo) == 0 {
				e.ReplyTo = ParseAddresses(val)
			}
		case "to":
			if len(e.To) == 0 {
				e.To = ParseAddresses(val)
			}
		case "cc":
			if len(e.CC) == 0 {
				e.CC = ParseAddresses(val)
			}
		case "bcc":
			if len(e.BCC) == 0 {
				e.BCC = ParseAddresses(val)
			}
		case "subject":
			e.Subject = val
		default:
			e.ExtraHeaders += v + "\r\n"
		}
	}
}

func (e *Email) GetMessage() string {
	if e.From == nil || e.From.Address == "" {
		panic("Invalid email: no sender")
	}

	msg := fmt.Sprintf("From: %s\r\n", e.From.StringFormat())
	// technically these can be different, we ignore this
	msg += fmt.Sprintf("Sender: %s\r\n", e.From.StringFormat())

	if len(e.ReplyTo) != 0 {
		msg += fmt.Sprintf("Reply-To: %s\r\n", FormatAddresses(&e.ReplyTo))
	}

	if len(e.To) == 0 || e.To[0].Address == "" {
		panic("Invalid email: no 'To'-recipient")
	}

	msg += fmt.Sprintf("To: %s\r\n", FormatAddresses(&e.To))

	if len(e.CC) != 0 {
		msg += fmt.Sprintf("CC: %s\r\n", FormatAddresses(&e.CC))
	}

	// *bcc explicitly not included*

	msg += fmt.Sprintf("Subject: %s\r\n", e.Subject)

	if e.ExtraHeaders != "" {
		msg += e.ExtraHeaders
	}

	msg += "\r\n"
	msg += e.Body

	return msg
}

func SendMail(e *Email) {
	smtp_hostname, _, err := net.SplitHostPort(SMTP_SERVER)
	if err != nil {
		// this errors if the port is not explicitly provided
		panic(err)
	}

	allRecAddrs := []string{}
	for _, v := range e.To {
		allRecAddrs = append(allRecAddrs, v.Address)
	}
	for _, v := range e.CC {
		allRecAddrs = append(allRecAddrs, v.Address)
	}
	for _, v := range e.BCC {
		allRecAddrs = append(allRecAddrs, v.Address)
	}

	auth := smtp.PlainAuth("", SMTP_USER, SMTP_PASSWORD, smtp_hostname)
	msg := e.GetMessage()

	err = smtp.SendMail(
		SMTP_SERVER,
		auth,
		e.From.Address,
		allRecAddrs,
		[]byte(msg),
	)

	if err != nil {
		// more advanced error logging?
		fmt.Println(err)
	}
}

func addToLog(logfile string, e *Email) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}

	file, err := os.OpenFile(logfile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(append(data, '\n'))
	if err != nil {
		return err
	}

	return nil
}

func main() {
	e := Email{Timestamp: time.Now()}
	e.PopulateFromArgs(os.Args[1:])
	e.PopulateFromStdin(os.Stdin)

	// TODO: do rewrites

	addToLog(LOG_FILE, &e)

	SendMail(&e)
}
