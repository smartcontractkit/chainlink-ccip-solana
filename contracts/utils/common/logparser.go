//revive:disable:var-naming // legacy package name
package common

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	bin "github.com/gagliardetto/binary"
)

// DecodeEventDataFromLog extracts and base64-decodes a single "Program data:" log line.
// Returns the decoded bytes and true if the line was a valid "Program data:" entry.
func DecodeEventDataFromLog(line string) ([]byte, bool) {
	trimmed := strings.TrimSpace(line)
	after, ok := strings.CutPrefix(trimmed, "Program data:")
	if !ok {
		return nil, false
	}
	encoded := strings.TrimSpace(after)
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, false
	}
	return data, true
}

// EventDataFromLogs extracts base64-decoded event data from Solana transaction
// log messages. It returns a slice of raw byte slices, one per "Program data:"
// entry, ready to be passed to a gobindings ParseEvent_* function.
func EventDataFromLogs(logs []string) [][]byte {
	var result [][]byte
	for _, log := range logs {
		if data, ok := DecodeEventDataFromLog(log); ok {
			result = append(result, data)
		}
	}
	return result
}

// ParseEventFromLogs iterates through log messages, decodes each "Program data:"
// entry, and returns the first event that parseFn successfully parses.
func ParseEventFromLogs[T any](logs []string, parseFn func([]byte) (*T, error)) (*T, error) {
	for _, data := range EventDataFromLogs(logs) {
		evt, err := parseFn(data)
		if err == nil {
			return evt, nil
		}
	}
	return nil, errors.New("event not found in logs")
}

// ParseMultipleEventsFromLogs iterates through log messages and returns all events
// that parseFn successfully parses.
func ParseMultipleEventsFromLogs[T any](logs []string, parseFn func([]byte) (*T, error)) ([]*T, error) {
	var results []*T
	for _, data := range EventDataFromLogs(logs) {
		evt, err := parseFn(data)
		if err == nil {
			results = append(results, evt)
		}
	}
	if len(results) == 0 {
		return nil, errors.New("event not found in logs")
	}
	return results, nil
}

func isEvent(event string, data []byte) bool {
	if len(data) < 8 {
		return false
	}
	d := Discriminator("event", event)
	return bytes.Equal(d, data[:8])
}

type AnchorInstruction struct {
	Name         string
	ProgramID    string
	Logs         []string
	ComputeUnits int
	InnerCalls   []*AnchorInstruction
	EventData    []*EventData
}

type EventMapping struct {
	Name string
	New  func() any
}

type EventData struct {
	Base64Data  string
	DecodedData []byte
	EventName   string
	Data        any // Decoded data using the provided type
}

func EventMappingFor[T any](name string) EventMapping {
	return EventMapping{
		Name: name,
		New: func() any {
			return new(T)
		},
	}
}

func NormalizeData(d []byte) []byte {
	if d == nil {
		return []byte{}
	}
	return d
}

// holds the state while parsing log lines
type parser struct {
	instructions       []*AnchorInstruction
	stack              []*AnchorInstruction
	currentInstruction *AnchorInstruction
	expectedEvents     []EventMapping

	// compiled regexes for performance
	programInvokeRegex  *regexp.Regexp
	programSuccessRegex *regexp.Regexp
	computeUnitsRegex   *regexp.Regexp
}

// creates a parser instance with precompiled regex
func newParser(expectedEvents []EventMapping) *parser {
	return &parser{
		instructions:        []*AnchorInstruction{},
		stack:               []*AnchorInstruction{},
		expectedEvents:      expectedEvents,
		programInvokeRegex:  regexp.MustCompile(`Program (\w+) invoke`),
		programSuccessRegex: regexp.MustCompile(`Program (\w+) success`),
		computeUnitsRegex:   regexp.MustCompile(`Program (\w+) consumed (\d+) of \d+ compute units`),
	}
}

func (p *parser) handleProgramInvokeLine(line string) bool {
	match := p.programInvokeRegex.FindStringSubmatch(line)
	if len(match) <= 1 {
		return false
	}

	newInstruction := &AnchorInstruction{
		ProgramID:    match[1],
		Name:         "",
		Logs:         []string{},
		ComputeUnits: 0,
		InnerCalls:   []*AnchorInstruction{},
		EventData:    []*EventData{},
	}

	if len(p.stack) == 0 {
		p.instructions = append(p.instructions, newInstruction)
	} else {
		p.stack[len(p.stack)-1].InnerCalls = append(p.stack[len(p.stack)-1].InnerCalls, newInstruction)
	}

	p.stack = append(p.stack, newInstruction)
	p.currentInstruction = newInstruction
	return true
}

// check if line is a "Program X success" line and updates stack
func (p *parser) handleProgramSuccessLine(line string) bool {
	match := p.programSuccessRegex.FindStringSubmatch(line)
	if len(match) <= 1 {
		return false
	}

	if len(p.stack) > 0 {
		p.stack = p.stack[:len(p.stack)-1] // pop
		if len(p.stack) > 0 {
			p.currentInstruction = p.stack[len(p.stack)-1]
		} else {
			p.currentInstruction = nil
		}
	}
	return true
}

func (p *parser) handleInstructionNameLine(line string) bool {
	if !strings.Contains(line, "Instruction:") {
		return false
	}
	if p.currentInstruction != nil {
		p.currentInstruction.Name = strings.TrimSpace(strings.Split(line, "Instruction:")[1])
	}
	return true
}

func (p *parser) handleProgramDataLine(line string) bool {
	decodedData, ok := DecodeEventDataFromLog(line)
	if !ok {
		return strings.Contains(line, "Program data:")
	}
	if p.currentInstruction == nil {
		return true // line recognized but no current instruction, do nothing
	}

	base64Data := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "Program data:"))

	for _, event := range p.expectedEvents {
		if isEvent(event.Name, decodedData) {
			obj := event.New()
			// NOTE: skipping the first 8 bytes which are the discriminator
			if err := bin.UnmarshalBorsh(&obj, decodedData[8:]); err != nil {
				continue
			}

			eventData := &EventData{
				Base64Data:  base64Data,
				DecodedData: decodedData,
				EventName:   event.Name,
				Data:        obj,
			}
			p.currentInstruction.EventData = append(p.currentInstruction.EventData, eventData)
		}
	}
	return true
}

func (p *parser) handleProgramLogLine(line string) bool {
	if !strings.HasPrefix(line, "Program log:") {
		return false
	}
	if p.currentInstruction != nil {
		logMessage := strings.TrimSpace(strings.TrimPrefix(line, "Program log:"))
		p.currentInstruction.Logs = append(p.currentInstruction.Logs, logMessage)
	}
	return true
}

func (p *parser) handleComputeUnitsLine(line string) bool {
	match := p.computeUnitsRegex.FindStringSubmatch(line)
	if len(match) <= 1 {
		return false
	}
	programID := match[1]
	units, _ := strconv.Atoi(match[2])

	// Find the instruction in the stack that matches this program ID
	for i := range slices.Backward(p.stack) {
		if p.stack[i].ProgramID == programID {
			p.stack[i].ComputeUnits = units
			break
		}
	}
	return true
}

func ParseLogMessages(logMessages []string, expectedEvents []EventMapping) []*AnchorInstruction {
	p := newParser(expectedEvents)

	for _, line := range logMessages {
		line = strings.TrimSpace(line)

		// Try each handler in turn
		if p.handleProgramInvokeLine(line) {
			continue
		}
		if p.handleProgramSuccessLine(line) {
			continue
		}
		if p.handleInstructionNameLine(line) {
			continue
		}
		if p.handleProgramDataLine(line) {
			continue
		}
		if p.handleProgramLogLine(line) {
			continue
		}
		if p.handleComputeUnitsLine(line) {
			continue
		}

		// if none matched, this line might be irrelevant
	}

	return p.instructions
}

// Pretty prints the given Anchor instructions.
// Example usage:
// parsed := utils.ParseLogMessages(result.Meta.LogMessages)
// output := utils.LogTxResult(parsed)
// t.Logf("Tx logs: %s", output)
func LogTxResult(instructions []*AnchorInstruction) string {
	var output strings.Builder

	var printInstruction func(*AnchorInstruction, int, string)
	printInstruction = func(instruction *AnchorInstruction, index int, indent string) {
		fmt.Fprintf(&output, "%sInstruction %d: %s\n", indent, index, instruction.Name)
		fmt.Fprintf(&output, "%s  Program ID: %s\n", indent, instruction.ProgramID)
		fmt.Fprintf(&output, "%s  Compute Units: %d\n", indent, instruction.ComputeUnits)

		// Print Events
		if len(instruction.EventData) > 0 {
			fmt.Fprintf(&output, "%s  Events:\n", indent)
			for _, event := range instruction.EventData {
				fmt.Fprintf(&output, "%s    Event: %s:\n", indent, event.EventName)
				fmt.Fprintf(&output, "%s      Base64Data: %+v\n", indent, event.Base64Data)
				fmt.Fprintf(&output, "%s      DecodedData: %+v\n", indent, event.DecodedData)
				fmt.Fprintf(&output, "%s      Data: %+v\n", indent, event.Data)
			}
		}

		fmt.Fprintf(&output, "%s  Logs:\n", indent)
		for _, log := range instruction.Logs {
			fmt.Fprintf(&output, "%s    %s\n", indent, log)
		}

		if len(instruction.InnerCalls) > 0 {
			fmt.Fprintf(&output, "%s  Inner Calls:\n", indent)
			for i, innerCall := range instruction.InnerCalls {
				printInstruction(innerCall, i+1, indent+"    ")
			}
		}
	}

	for i, instruction := range instructions {
		printInstruction(instruction, i+1, "")
	}

	return output.String()
}
