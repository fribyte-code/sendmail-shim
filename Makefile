# variables compiled directly in the final statically linked binary
# requires this makefile to be run with these as availiable environment variables
ifndef SMTP_SERVER
$(error SMTP_SERVER build-time variable is not set)
endif
ifndef SMTP_USER
$(error SMTP_USER build-time variable is not set)
endif
ifndef SMTP_PASSWORD
$(error SMTP_PASSWORD build-time variable is not set)
endif
ifndef LOG_FILE
$(error LOG_FILE build-time variable is not set)
endif


sendmail: shim.go
	CGO_ENABLED=0 go build -o sendmail -ldflags \
		"-X main.SMTP_SERVER=$(SMTP_SERVER) -X main.SMTP_USER=$(SMTP_USER) -X main.SMTP_PASSWORD=$(SMTP_PASSWORD) -X main.LOG_FILE=$(LOG_FILE)" \
		shim.go

# Credit to this stackoverflow answer https://stackoverflow.com/a/14061796
ifeq (run, $(firstword $(MAKECMDGOALS)))
  RUN_ARGS := $(wordlist 2, $(words $(MAKECMDGOALS)), $(MAKECMDGOALS))
  $(eval $(RUN_ARGS):;@:)
endif

# I recommend running this command with --always-make to ensure most recent code is running
.PHONY: run
run: sendmail
	./sendmail $(RUN_ARGS)

