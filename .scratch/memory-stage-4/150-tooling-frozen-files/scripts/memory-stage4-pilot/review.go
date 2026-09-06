package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

type reviewInput struct {
	Command      string              `json:"command"`
	Candidate    memory.CandidateRef `json:"candidate"`
	Scope        string              `json:"scope_key"`
	GenerationID string              `json:"generation_id"`
	Action       string              `json:"action"`
	Reason       string              `json:"reason"`
	Useful       *bool               `json:"useful"`
	Receipt      string              `json:"review_receipt"`
}
type reviewObservation struct {
	Version             string              `json:"version"`
	Operator            string              `json:"operator_self_attestation"`
	ConfigurationSHA256 string              `json:"configuration_sha256"`
	Candidate           memory.CandidateRef `json:"candidate"`
	Scope               string              `json:"scope_key"`
	GenerationID        string              `json:"generation_id"`
	StartedAt           time.Time           `json:"started_at"`
	FinishedAt          time.Time           `json:"finished_at"`
	ActiveNanos         int64               `json:"active_nanos"`
	Action              string              `json:"action"`
	Reason              string              `json:"reason"`
	Useful              *bool               `json:"owner_reported_useful"`
	Receipt             string              `json:"external_review_receipt"`
	ReceiptVerified     bool                `json:"receipt_verified"`
}
type reviewClock struct {
	current     *reviewObservation
	activeSince time.Time
	active      bool
}

func (c *reviewClock) apply(input reviewInput, now time.Time) (*reviewObservation, error) {
	switch input.Command {
	case "start":
		if c.current != nil {
			return nil, errors.New("finish the current candidate first")
		}
		if input.Candidate.ID == "" || input.Candidate.InterpretationRevision < 1 || input.Candidate.ReviewRevision < 0 || input.Scope == "" || input.GenerationID == "" {
			return nil, errors.New("start requires exact candidate revisions, scope and generation")
		}
		c.current = &reviewObservation{Version: "memory-stage4-owner-review-observation-v1", Candidate: input.Candidate, Scope: input.Scope, GenerationID: input.GenerationID, StartedAt: now.UTC()}
		c.activeSince = now
		c.active = true
	case "pause":
		if c.current == nil || !c.active {
			return nil, errors.New("no active candidate")
		}
		c.current.ActiveNanos += now.Sub(c.activeSince).Nanoseconds()
		c.active = false
	case "resume":
		if c.current == nil || c.active {
			return nil, errors.New("no paused candidate")
		}
		c.activeSince = now
		c.active = true
	case "finish":
		if c.current == nil {
			return nil, errors.New("no current candidate")
		}
		if input.Action != "accept" && input.Action != "edit" && input.Action != "reject" && input.Action != "defer" {
			return nil, errors.New("finish action must be accept, edit, reject, or defer")
		}
		if strings.TrimSpace(input.Reason) == "" || len(input.Reason) > 4096 {
			return nil, errors.New("finish requires a reason of at most 4096 bytes")
		}
		if input.Action != "defer" && strings.TrimSpace(input.Receipt) == "" {
			return nil, errors.New("state-changing decisions require an external review receipt reference")
		}
		if c.active {
			c.current.ActiveNanos += now.Sub(c.activeSince).Nanoseconds()
		}
		r := c.current
		r.FinishedAt = now.UTC()
		r.Action = input.Action
		r.Reason = input.Reason
		r.Useful = input.Useful
		r.Receipt = input.Receipt
		c.current = nil
		c.active = false
		return r, nil
	default:
		return nil, errors.New("unknown review-session command")
	}
	return nil, nil
}

func reviewCommand(args []string, input io.Reader, output io.Writer) error {
	f := flag.NewFlagSet("review-session", flag.ContinueOnError)
	operator := f.String("operator", "", "human operator's explicit self-attestation")
	configuration := f.String("configuration-sha256", "", "SHA-256 of the pinned pilot configuration")
	path := f.String("output", "", "new private JSONL observation file")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || strings.TrimSpace(*operator) == "" || len(*operator) > 128 || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(*configuration) || *path == "" {
		return errors.New("review-session requires operator, exact configuration SHA-256 and output")
	}
	file, err := os.OpenFile(*path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	fmt.Fprintln(output, "Timing is explicit: start, pause, resume, finish JSON commands. Decisions occur in Evie's review surface. Receipts and identity are self-reported until separately checked. EOF during a candidate discards its incomplete timing.")
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 16384)
	clock := &reviewClock{}
	for scanner.Scan() {
		var command reviewInput
		decoder := json.NewDecoder(strings.NewReader(scanner.Text()))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&command); err != nil {
			return err
		}
		var extra any
		if !errors.Is(decoder.Decode(&extra), io.EOF) {
			return errors.New("one JSON command per line required")
		}
		observation, e := clock.apply(command, time.Now())
		if e != nil {
			return e
		}
		if observation != nil {
			observation.Operator = *operator
			observation.ConfigurationSHA256 = *configuration
			if err = json.NewEncoder(file).Encode(observation); err != nil {
				return err
			}
			if err = file.Sync(); err != nil {
				return err
			}
			fmt.Fprintln(output, "Observation saved; external receipt verification remains required.")
		} else if command.Command == "pause" {
			fmt.Fprintln(output, "Timing paused.")
		} else if command.Command == "resume" {
			fmt.Fprintln(output, "Timing resumed.")
		}
	}
	if err = scanner.Err(); err != nil {
		return err
	}
	if clock.current != nil {
		return errors.New("incomplete candidate timing was not saved")
	}
	return nil
}
