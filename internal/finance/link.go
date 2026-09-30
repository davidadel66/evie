package finance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type accountLinkService interface {
	InspectAccounts(context.Context) (SpendingAccountsReport, error)
	StartAccountLink(context.Context) (SpendingAccountLink, error)
	CompleteAccountLink(context.Context, string) (SpendingAccountLinkResult, error)
	CancelAccountLink(context.Context, string) error
}

// Link runs or resumes the shared durable Hosted Link flow. A new initial Link
// can create a separate Item for an already connected bank; it is not a repair
// flow. Only the hosted URL and safe status messages are printed.
func Link() error {
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(signalCtx, 35*time.Minute)
	defer cancel()
	return linkWithService(ctx, NewSpendingService(), os.Stdout, 3*time.Second)
}

func linkWithService(ctx context.Context, service accountLinkService, output io.Writer, pollInterval time.Duration) error {
	var link SpendingAccountLink
	interrupted := func() error {
		if errors.Is(ctx.Err(), context.Canceled) {
			if link.ID != "" {
				cleanupCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				defer stop()
				// A concurrent exchange may already have saved the connection.
				// Never describe cancellation of this wait as provider rollback.
				_ = service.CancelAccountLink(cleanupCtx, link.ID)
			}
			return fmt.Errorf("bank connection interrupted; check saved accounts before starting again: %w", ctx.Err())
		}
		return fmt.Errorf("stopped waiting for bank connection; run finance link to resume: %w", ctx.Err())
	}
	if ctx.Err() != nil {
		return interrupted()
	}
	var err error
	link, err = service.StartAccountLink(ctx)
	if errors.Is(err, ErrSpendingLinkInProgress) {
		// An expired hosted URL may still have an unconsumed completion result.
		// Resolve that saved flow before asking Plaid for another one.
		report, inspectErr := service.InspectAccounts(ctx)
		if inspectErr == nil && report.PendingLink != nil {
			link, err = *report.PendingLink, nil
		}
	}
	if err != nil {
		if ctx.Err() != nil {
			return interrupted()
		}
		if errors.Is(err, ErrSpendingLinkInProgress) {
			return ErrSpendingLinkInProgress
		}
		return ErrSpendingLinkUnavailable
	}
	if link.ExpiresAt.After(time.Now()) {
		if _, err := fmt.Fprintf(output, "Open this in your browser to link your bank:\n%s\nWaiting for the connection to finish...\n", link.HostedURL); err != nil {
			return errors.New("could not write bank connection instructions")
		}
	} else if _, err := fmt.Fprintln(output, "Checking a saved bank connection..."); err != nil {
		return errors.New("could not write bank connection status")
	}
	for {
		if ctx.Err() != nil {
			return interrupted()
		}
		result, err := service.CompleteAccountLink(ctx, link.ID)
		if err != nil && !errors.Is(err, ErrSpendingLinkInProgress) {
			if ctx.Err() != nil {
				return interrupted()
			}
			return ErrSpendingLinkUnavailable
		}
		if err == nil {
			switch result.Status {
			case "linked":
				if _, err := fmt.Fprintln(output, "Linked! Bank connection saved."); err != nil {
					return errors.New("bank connection saved, but its status could not be printed")
				}
				return nil
			case "expired":
				return errors.New("bank connection expired; run finance link to start again")
			case "cancelled":
				return errors.New("bank connection was cancelled")
			case "failed":
				return errors.New("bank connection could not be completed; check saved accounts before trying again")
			case "pending":
			default:
				return ErrSpendingLinkUnavailable
			}
		}
		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return interrupted()
		case <-timer.C:
		}
	}
}
